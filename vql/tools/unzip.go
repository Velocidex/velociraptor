//+build extras

package tools

import (
	"context"
	"os"
	"path/filepath"

	"github.com/Velocidex/ordereddict"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	"www.velocidex.com/golang/velociraptor/glob"
	"www.velocidex.com/golang/velociraptor/utils"
	vql_subsystem "www.velocidex.com/golang/velociraptor/vql"
	"www.velocidex.com/golang/vfilter"
)

type UnzipResponse struct {
	OriginalPath string
	NewPath      string
	Size         int64
}

type UnzipPluginArgs struct {
	Filename        string `vfilter:"required,field=filename,doc=File to unzip."`
	OutputDirectory string `vfilter:"required,field=output_directory,doc=Where to unzip to"`
}

type UnzipPlugin struct{}

func (self UnzipPlugin) Call(
	ctx context.Context,
	scope vfilter.Scope,
	args *ordereddict.Dict) <-chan vfilter.Row {
	output_chan := make(chan vfilter.Row)
	globber := make(glob.Globber)
	go func() {
		defer close(output_chan)
		config_obj, ok := vql_subsystem.GetServerConfig(scope)
		if !ok {
			config_obj = &config_proto.Config{}
		}
		arg := &UnzipPluginArgs{}
		err := vfilter.ExtractArgs(scope, args, arg)
		if err != nil {
			scope.Log("unzip: %s", err.Error())
			return
		}
		accessor, err := glob.GetAccessor("zip", scope)
		if err != nil {
			scope.Log("unzip: %v", err)
			return
		}

		root := ""
		item_root, _, _ := accessor.GetRoot(arg.Filename)
		root = item_root
		err = globber.Add("#**{50}", accessor.PathSplit)
		if err != nil {
			scope.Log("unzip: %v", err)
			return
		}

		file_chan := globber.ExpandWithContext(
			ctx, config_obj, root, accessor)
		for f := range file_chan {
			select {
			case <-ctx.Done():
				return

			default:
				{
					_, item_path, _ := accessor.GetRoot(f.FullPath())
					if !f.IsDir() && !f.IsLink() {
						new_path := filepath.Join(arg.OutputDirectory, item_path)
						fd, err := accessor.Open(f.FullPath())
						if err != nil {
							scope.Log("unzip: %v", err)
							return
						}

						if err := os.MkdirAll(filepath.Dir(new_path), 0700); err != nil {
							scope.Log("unzip: %v", err)
							return
						}

						fd2, err := os.OpenFile(new_path, os.O_CREATE|os.O_WRONLY, 0700)
						defer fd2.Close()

						if err != nil {
							scope.Log("unzip: %v", err)
							return
						}

						_, err = utils.Copy(ctx, fd2, fd)
						if err != nil {
							scope.Log("unzip: %v", err)
							return
						}

						output := &UnzipResponse{
							OriginalPath: f.FullPath(),
							NewPath:      new_path,
							Size:         f.Size(),
						}
						output_chan <- output
					}
				}
			}
		}
	}()
	return output_chan
}

func (self UnzipPlugin) Name() string {
	return "unzip"
}

func (self UnzipPlugin) Info(scope vfilter.Scope, type_map *vfilter.TypeMap) *vfilter.PluginInfo {
	return &vfilter.PluginInfo{
		Name:    "unzip",
		Doc:     "Unzips a directory",
		ArgType: type_map.AddType(scope, &UnzipPluginArgs{}),
	}
}

func init() {
	vql_subsystem.RegisterPlugin(&UnzipPlugin{})
}
