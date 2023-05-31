package parsers

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"syscall"

	"github.com/Velocidex/ordereddict"
	"github.com/syndtr/goleveldb/leveldb"
	"github.com/syndtr/goleveldb/leveldb/opt"
	"www.velocidex.com/golang/velociraptor/accessors"
	"www.velocidex.com/golang/velociraptor/acls"
	utils "www.velocidex.com/golang/velociraptor/utils"
	"www.velocidex.com/golang/velociraptor/vql"
	vql_subsystem "www.velocidex.com/golang/velociraptor/vql"
	"www.velocidex.com/golang/velociraptor/vql/filesystem"
	vfilter "www.velocidex.com/golang/vfilter"
	"www.velocidex.com/golang/vfilter/arg_parser"
)

type LevelDBPluginArgs struct {
	Filename *accessors.OSPath `vfilter:"optional,field=file, doc=The path to the leveldb file."`
	Accessor string            `vfilter:"optional,field=accessor,doc=The accessor to use."`
}

type LevelDBPlugin struct{}

func (self LevelDBPlugin) Call(
	ctx context.Context,
	scope vfilter.Scope,
	args *ordereddict.Dict) <-chan vfilter.Row {
	output_chan := make(chan vfilter.Row)
	go func() {
		defer close(output_chan)
		defer utils.RecoverVQL(scope)

		arg := &LevelDBPluginArgs{}
		err := arg_parser.ExtractArgsWithContext(ctx, scope, args, arg)
		if err != nil {
			scope.Log("leveldb: %v", err)
			return
		}

		if arg.Accessor == "" {
			arg.Accessor = "auto"
		}

		err = vql_subsystem.CheckFilesystemAccess(scope, arg.Accessor)
		if err != nil {
			scope.Log("leveldb: %s", err)
			return
		}

		var db *leveldb.DB
		switch arg.Accessor {
		case "", "auto", "file":
			db, err = leveldb.OpenFile(arg.Filename.String(), &opt.Options{
				ReadOnly: true,
				Strict:   opt.NoStrict,
			})
			if err != nil {
				if !retriableError(err) {
					scope.Log("leveldb: %v", err)
					return
				}
				scope.Log("DEBUG:leveldb: Directly opening file faild with %v, retrying on a local copy", err)
				local_path, err1 := maybeMakeLocalCopy(ctx, scope, arg)
				if err1 != nil {
					scope.Log("leveldb: %v", err)
					scope.Log("leveldb: %v", err1)
					return
				}

				// Try again with the copy
				db, err = leveldb.OpenFile(local_path, &opt.Options{
					ReadOnly: true,
					Strict:   opt.NoStrict,
				})
				if err != nil {
					scope.Log("leveldb: %v", err)
					return
				}
			}

			// For other accessors we just always make a copy.
		default:
			local_path, err := maybeMakeLocalCopy(ctx, scope, arg)
			if err != nil {
				scope.Log("leveldb: %v", err)
				return
			}

			// Try again with the copy
			db, err = leveldb.OpenFile(local_path, &opt.Options{
				ReadOnly: true,
				Strict:   opt.NoStrict,
			})
			if err != nil {
				scope.Log("leveldb: %v", err)
				return
			}
		}
		defer db.Close()

		iter := db.NewIterator(nil, nil)
		for iter.Next() {
			key := iter.Key()
			value := iter.Value()
			select {
			case <-ctx.Done():
				return
			case output_chan <- ordereddict.NewDict().
				Set("Key", string(key)).
				Set("Value", string(value)):
			}
		}
		iter.Release()
		err = iter.Error()
		if err != nil {
			scope.Log("leveldb: %v", err)
		}
	}()
	return output_chan
}

func (self LevelDBPlugin) Info(scope vfilter.Scope, type_map *vfilter.TypeMap) *vfilter.PluginInfo {
	return &vfilter.PluginInfo{
		Name:     "leveldb",
		Doc:      "Enumerate all items in a level db database",
		ArgType:  type_map.AddType(scope, &LevelDBPluginArgs{}),
		Metadata: vql.VQLMetadata().Permissions(acls.FILESYSTEM_READ).Build(),
	}
}

// Maybe make a local copy of the database files.
func maybeMakeLocalCopy(
	ctx context.Context, scope vfilter.Scope,
	arg *LevelDBPluginArgs) (string, error) {
	accessor, err := accessors.GetAccessor(arg.Accessor, scope)
	if err != nil {
		return "", err
	}

	files, err := accessor.ReadDirWithOSPath(arg.Filename)
	if err != nil {
		return "", err
	}

	// Create a temp directory to contain all the files.
	tmpdir_any := (&filesystem.TempdirFunction{}).Call(ctx, scope, ordereddict.NewDict())
	tmpdir, ok := tmpdir_any.(string)
	if !ok {
		return "", errors.New("Unable to create tempdir")
	}

	total_bytes := 0
	for _, f_info := range files {
		in_fd, err := accessor.OpenWithOSPath(f_info.OSPath())
		if err != nil {
			return "", err
		}

		out_fd, err := os.OpenFile(filepath.Join(tmpdir, f_info.Name()),
			os.O_RDWR|os.O_CREATE|os.O_TRUNC, 0700)
		if err != nil {
			in_fd.Close()
			return "", err
		}

		n, _ := utils.Copy(ctx, out_fd, in_fd)
		total_bytes += n

		in_fd.Close()
		out_fd.Close()
	}

	scope.Log("INFO:leveldb: Copied db %v with accessor %v to local "+
		"tmp directory %v (Copied %v files, %v bytes)\n",
		arg.Filename, arg.Accessor, tmpdir, len(files), total_bytes)
	return tmpdir, nil
}

// Retry if the error is SHARING_VIOLATION on windows.
func retriableError(err error) bool {
	errno, ok := err.(syscall.Errno)
	return ok && errno == 32
}

func init() {
	vql_subsystem.RegisterPlugin(&LevelDBPlugin{})
}
