package crypto

import (
	"context"
	"io"

	"github.com/Velocidex/ordereddict"
	"www.velocidex.com/golang/velociraptor/accessors"
	"www.velocidex.com/golang/velociraptor/acls"
	"www.velocidex.com/golang/velociraptor/crypto/storage"
	"www.velocidex.com/golang/velociraptor/utils"
	"www.velocidex.com/golang/velociraptor/vql"
	vql_subsystem "www.velocidex.com/golang/velociraptor/vql"
	"www.velocidex.com/golang/vfilter"
	"www.velocidex.com/golang/vfilter/arg_parser"
)

type ReaderAtterCloser struct {
	io.ReaderAt
	fd utils.Closer
}

func (self *ReaderAtterCloser) Close() error {
	return self.fd.Close()
}

type ReadCryptFilePluginArgs struct {
	Filename *accessors.OSPath `vfilter:"required,field=filename,doc=Path to the file to write"`
	Accessor string            `vfilter:"optional,field=accessor,doc=The accessor to use"`
}

type ReadCryptFilePlugin struct{}

func (self ReadCryptFilePlugin) Call(
	ctx context.Context,
	scope vfilter.Scope,
	args *ordereddict.Dict) <-chan vfilter.Row {
	output_chan := make(chan vfilter.Row)

	go func() {
		defer close(output_chan)
		defer vql_subsystem.RegisterMonitor(ctx, "read_crypto_file", args)()

		arg := &ReadCryptFilePluginArgs{}
		err := arg_parser.ExtractArgsWithContext(ctx, scope, args, arg)
		if err != nil {
			scope.Log("read_crypto_file: %v", err)
			return
		}

		err = vql_subsystem.CheckAccess(scope, acls.FILESYSTEM_READ)
		if err != nil {
			scope.Log("read_crypto_file: %v", err)
			return
		}

		config_obj, ok := vql_subsystem.GetServerConfig(scope)
		if !ok {
			scope.Log("read_crypto_file: Must have access to server configuration", err)
			return
		}

		accessor, err := accessors.GetAccessor(arg.Accessor, scope)
		if err != nil {
			scope.Log("read_crypto_file: %v", err)
			return
		}

		fd, err := accessor.OpenWithOSPath(arg.Filename)
		if err != nil {
			scope.Log("read_crypto_file: Unable to open file %s: %v",
				arg.Filename.String(), err)
			return
		}
		defer fd.Close()

		reader, err := storage.NewCryptoFileReader(ctx,
			config_obj,
			&ReaderAtterCloser{
				ReaderAt: utils.MakeReaderAtter(fd),
				fd:       fd,
			})
		if err != nil {
			scope.Log("write_crypto_file: %v", err)
			return
		}
		defer reader.Close()

		for packet := range reader.Parse(ctx) {
			if packet.VQLResponse == nil {
				continue
			}

			row, err := utils.ParseJsonToObject(
				[]byte(packet.VQLResponse.JSONLResponse))
			if err != nil {
				continue
			}

			select {
			case <-ctx.Done():
				return
			case output_chan <- row:
			}
		}
	}()

	return output_chan
}

func (self ReadCryptFilePlugin) Info(scope vfilter.Scope, type_map *vfilter.TypeMap) *vfilter.PluginInfo {
	return &vfilter.PluginInfo{
		Name:     "read_crypto_file",
		Doc:      "Read a previously stored encrypted local storage file.",
		ArgType:  type_map.AddType(scope, &ReadCryptFilePluginArgs{}),
		Metadata: vql.VQLMetadata().Permissions(acls.FILESYSTEM_READ).Build(),
	}
}

func init() {
	vql_subsystem.RegisterPlugin(&ReadCryptFilePlugin{})
}
