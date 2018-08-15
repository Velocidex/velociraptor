package vql

import (
	"context"
	"github.com/shirou/gopsutil/disk"
	"www.velocidex.com/golang/velociraptor/glob"
	"www.velocidex.com/golang/vfilter"
)

type GlobPluginArgs struct {
	Globs []string `vfilter:"required,field=globs"`
}

type GlobPlugin struct{}

func (self GlobPlugin) Call(
	ctx context.Context,
	scope *vfilter.Scope,
	args *vfilter.Dict) <-chan vfilter.Row {
	globber := make(glob.Globber)
	output_chan := make(chan vfilter.Row)
	arg := &GlobPluginArgs{}
	err := vfilter.ExtractArgs(scope, args, arg)
	if err != nil {
		scope.Log("%s: %s", self.Name(), err.Error())
		close(output_chan)
		return output_chan
	}

	accessor := &glob.OSFileSystemAccessor{}
	for _, item := range arg.Globs {
		globber.Add(item, "/")
	}

	go func() {
		defer close(output_chan)
		file_chan := globber.ExpandWithContext(
			ctx, accessor.PathSep(), accessor)
		for {
			select {
			case <-ctx.Done():
				return

			case f, ok := <-file_chan:
				if !ok {
					return
				}
				output_chan <- f
			}
		}
	}()

	return output_chan
}

func (self GlobPlugin) Name() string {
	return "glob"
}

func (self GlobPlugin) Info(type_map *vfilter.TypeMap) *vfilter.PluginInfo {
	return &vfilter.PluginInfo{
		Name:    "glob",
		Doc:     "Retrieve files based on a list of glob expressions",
		RowType: type_map.AddType(glob.OSFileInfo{}),
	}
}

type StatArgs struct {
	Filename string `vfilter:"required,field=filename"`
}

func init() {
	exportedPlugins = append(exportedPlugins,
		&GlobPlugin{},
		vfilter.GenericListPlugin{
			PluginName: "filesystems",
			Function: func(
				scope *vfilter.Scope,
				args *vfilter.Dict) []vfilter.Row {
				var result []vfilter.Row
				partitions, err := disk.Partitions(true)
				if err == nil {
					for _, item := range partitions {
						result = append(result, item)
					}
				}
				return result
			},
			RowType: disk.PartitionStat{},
		},
		vfilter.GenericListPlugin{
			PluginName: "stat",
			Function: func(
				scope *vfilter.Scope,
				args *vfilter.Dict) []vfilter.Row {
				var result []vfilter.Row

				arg := &StatArgs{}
				err := vfilter.ExtractArgs(scope, args, arg)
				if err != nil {
					scope.Log("%s: %s", "stat", err.Error())
					return result
				}

				accessor := &glob.OSFileSystemAccessor{}
				f, err := accessor.Lstat(arg.Filename)
				if err == nil {
					result = append(result, f)
				}
				return result
			},
			RowType: &glob.OSFileInfo{},
		})
}
