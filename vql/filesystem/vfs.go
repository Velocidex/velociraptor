package filesystem

import (
	"context"

	"github.com/Velocidex/ordereddict"
	"www.velocidex.com/golang/velociraptor/accessors"
	"www.velocidex.com/golang/velociraptor/acls"
	"www.velocidex.com/golang/velociraptor/services"
	"www.velocidex.com/golang/velociraptor/vql"
	vql_subsystem "www.velocidex.com/golang/velociraptor/vql"
	"www.velocidex.com/golang/vfilter"
	"www.velocidex.com/golang/vfilter/arg_parser"
)

type VFSListDirectoryPluginArgs struct {
	Path       *accessors.OSPath `vfilter:"optional,field=path,doc=The directory to refresh."`
	Components []string          `vfilter:"optional,field=components,doc=Alternatively a list of path components can be given."`
	Accessor   string            `vfilter:"optional,field=accessor,doc=An accessor to use."`
	Depth      int64             `vfilter:"optional,field=depth,doc=Depth of directory to list (default 0)."`
}

type VFSListDirectoryPlugin struct{}

func (self VFSListDirectoryPlugin) Call(
	ctx context.Context,
	scope vfilter.Scope,
	args *ordereddict.Dict) <-chan vfilter.Row {
	output_chan := make(chan vfilter.Row)

	go func() {
		defer close(output_chan)
		defer vql_subsystem.RegisterMonitor(ctx, "vfs_ls", args)()

		arg := &VFSListDirectoryPluginArgs{}
		err := arg_parser.ExtractArgsWithContext(ctx, scope, args, arg)
		if err != nil {
			scope.Log("vfs_ls: %v", err)
			return
		}

		if arg.Accessor == "" {
			arg.Accessor = "file"
		}

		if arg.Path == nil {
			accessor, err := accessors.GetAccessor(arg.Accessor, scope)
			if err != nil {
				scope.Log("vfs_ls: %v", err)
				return
			}
			arg.Path, err = accessor.ParsePath("")
			if err != nil {
				scope.Log("vfs_ls: %v", err)
				return
			}
		}

		if len(arg.Components) > 0 {
			arg.Path = arg.Path.Append(arg.Components...)
		}

		stats := &services.VFSPartition{}
		listDir(ctx, scope, stats, output_chan, arg.Path,
			arg.Accessor, arg.Depth, 0)
	}()

	return output_chan
}

func listDir(
	ctx context.Context,
	scope vfilter.Scope,
	stats *services.VFSPartition,
	output_chan chan<- vfilter.Row,
	path *accessors.OSPath,
	accessor_name string,
	max_depth int64, depth int64) {

	if depth > max_depth {
		return
	}

	// Start counting the rows in this directory.
	stats.StartIdx = stats.EndIdx

	accessor, err := accessors.GetAccessor(accessor_name, scope)
	if err != nil {
		scope.Log("vfs_ls: %v", err)
		return
	}

	files, err := accessor.ReadDirWithOSPath(path)
	if err != nil {
		scope.Log("vfs_ls: %v", err)
		return
	}

	var directories []*accessors.OSPath

	for _, f := range files {
		// Remember all the directories so we can recurse if we need
		// to.
		if max_depth > depth && f.Mode().IsDir() {
			directories = append(directories, f.OSPath())
		}

		select {
		case <-ctx.Done():
			return

		case output_chan <- &services.VFSListRow{
			FullPath:   f.FullPath(),
			OSPath:     f.OSPath(),
			Components: f.OSPath().Components,
			Accessor:   accessor_name,
			Data:       f.Data(),
			Stats:      nil,
			Name:       f.Name(),
			Size:       f.Size(),
			Mode:       f.Mode().String(),
			Mtime:      f.Mtime(),
			Atime:      f.Atime(),
			Ctime:      f.Ctime(),
			Btime:      f.Btime(),
			Idx:        stats.EndIdx,
		}:
			stats.EndIdx++
		}
	}

	// Send a stats message
	output_chan <- &services.VFSListRow{
		Components: path.Components,
		Accessor:   accessor_name,
		Stats: &services.VFSPartition{
			StartIdx: stats.StartIdx,
			EndIdx:   stats.EndIdx,
		}}

	// Now recurse into all subdirectories.
	for _, dir_path := range directories {
		listDir(ctx, scope, stats, output_chan,
			dir_path, accessor_name, max_depth, depth+1)
	}
}

func (self VFSListDirectoryPlugin) Info(scope vfilter.Scope, type_map *vfilter.TypeMap) *vfilter.PluginInfo {
	return &vfilter.PluginInfo{
		Name:     "vfs_ls",
		Doc:      "List directory and build a VFS object",
		ArgType:  type_map.AddType(scope, &VFSListDirectoryPluginArgs{}),
		Version:  1,
		Metadata: vql.VQLMetadata().Permissions(acls.FILESYSTEM_READ).Build(),
	}
}

func init() {
	vql_subsystem.RegisterPlugin(&VFSListDirectoryPlugin{})
}
