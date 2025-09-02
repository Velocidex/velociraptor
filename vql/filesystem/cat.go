package filesystem

import (
	"context"
	"io"
	"os"
	"time"

	"github.com/Velocidex/ordereddict"
	"github.com/go-errors/errors"
	"www.velocidex.com/golang/velociraptor/accessors"
	"www.velocidex.com/golang/velociraptor/acls"
	"www.velocidex.com/golang/velociraptor/utils"
	"www.velocidex.com/golang/velociraptor/vql"
	vql_subsystem "www.velocidex.com/golang/velociraptor/vql"
	vfilter "www.velocidex.com/golang/vfilter"
	"www.velocidex.com/golang/vfilter/arg_parser"
)

type CatPluginArgs struct {
	Filename *accessors.OSPath `vfilter:"required,field=filename,doc=File to open."`
	Accessor string            `vfilter:"optional,field=accessor,doc=An accessor to use."`
	Chunk    int               `vfilter:"optional,field=chunk,doc=length of each chunk to read from the file."`
	Timeout  int               `vfilter:"optional,field=timeout,doc=If specified we abort reading after this much time."`
}

type CatPlugin struct{}

func (self CatPlugin) Call(
	ctx context.Context,
	scope vfilter.Scope,
	args *ordereddict.Dict) <-chan vfilter.Row {
	output_chan := make(chan vfilter.Row)

	go func() {
		defer close(output_chan)
		defer vql_subsystem.RegisterMonitor(ctx, "cat", args)()

		arg := &CatPluginArgs{}
		err := arg_parser.ExtractArgsWithContext(ctx, scope, args, arg)
		if err != nil {
			scope.Log("cat: %v", err)
			return
		}

		if arg.Chunk == 0 {
			arg.Chunk = 4 * 1024 * 1024
		}

		// Cap the size of the buffer to something reasonable.
		if arg.Chunk > 100*1024*1024 {
			arg.Chunk = 100 * 1024 * 1024
		}

		accessor, err := accessors.GetAccessor(arg.Accessor, scope)
		if err != nil {
			scope.Log("cat: %v", err)
			return
		}

		raw_accessor, ok := accessor.(accessors.RawFileAPIAccessor)
		if !ok {
			scope.Log("cat: accessor has no underlying file")
			return
		}

		underlying_file, err := raw_accessor.GetUnderlyingAPIFilename(arg.Filename)
		if err != nil {
			scope.Log("cat: %v", err)
			return
		}

		fd, err := os.Open(underlying_file)
		if err != nil {
			scope.Log("cat: %v", err)
			return
		}
		defer fd.Close()

		if arg.Timeout > 0 {
			go func() {
				utils.SleepWithCtx(ctx, time.Second*time.Duration(arg.Timeout))
				fd.Close()
			}()
		}

		for {
			buffer := make([]byte, arg.Chunk)
			n, err := fd.Read(buffer)
			if errors.Is(err, io.EOF) || n == 0 {
				return
			}

			if err != nil {
				scope.Log("cat: %v", err)
				return
			}

			select {
			case <-ctx.Done():
				return
			case output_chan <- ordereddict.NewDict().
				Set("Data", buffer[:n]):
			}
		}
	}()

	return output_chan
}

func (self CatPlugin) Info(scope vfilter.Scope, type_map *vfilter.TypeMap) *vfilter.PluginInfo {
	return &vfilter.PluginInfo{
		Name: "cat",
		Doc: `Read files in chunks.

This is mostly useful for character devices on Linux or special files which can not be read in blocks.`,
		ArgType:  type_map.AddType(scope, &CatPluginArgs{}),
		Metadata: vql.VQLMetadata().Permissions(acls.FILESYSTEM_READ).Build(),
	}
}

func init() {
	vql_subsystem.RegisterPlugin(&CatPlugin{})
}
