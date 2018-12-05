package filesystem

import (
	"context"

	"github.com/shirou/gopsutil/disk"
	"www.velocidex.com/golang/velociraptor/glob"
	vql_subsystem "www.velocidex.com/golang/velociraptor/vql"
	"www.velocidex.com/golang/vfilter"
)

type GlobPluginArgs struct {
	Globs    []string `vfilter:"required,field=globs"`
	Accessor string   `vfilter:"optional,field=accessor"`
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
		scope.Log("glob: %s", err.Error())
		close(output_chan)
		return output_chan
	}

	accessor := glob.GetAccessor(arg.Accessor, ctx)
	root := ""
	for _, item := range arg.Globs {
		item_root, item_path, _ := accessor.GetRoot(item)
		if root != "" && root != item_root {
			scope.Log("glob: %s: Must use the same root for "+
				"all globs. Skipping.", item)
			continue
		}
		root = item_root
		globber.Add(item_path, accessor.PathSplit())
	}

	go func() {
		defer close(output_chan)
		file_chan := globber.ExpandWithContext(
			ctx, root, accessor)
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

func (self GlobPlugin) Info(scope *vfilter.Scope, type_map *vfilter.TypeMap) *vfilter.PluginInfo {
	return &vfilter.PluginInfo{
		Name:    "glob",
		Doc:     "Retrieve files based on a list of glob expressions",
		RowType: type_map.AddType(scope, glob.NewVirtualDirectoryPath("", nil)),
		ArgType: type_map.AddType(scope, &GlobPluginArgs{}),
	}
}

type ReadFileArgs struct {
	Chunk     int      `vfilter:"optional,field=chunk"`
	MaxLength int      `vfilter:"optional,field=max_length"`
	Filenames []string `vfilter:"required,field=filenames"`
	Accessor  string   `vfilter:"optional,field=accessor"`
}

type ReadFileResponse struct {
	Data     string
	Offset   int64
	Filename string
}

type ReadFilePlugin struct{}

func (self ReadFilePlugin) processFile(
	ctx context.Context,
	scope *vfilter.Scope,
	arg *ReadFileArgs,
	file string,
	output_chan chan vfilter.Row) {
	total_len := int64(0)
	accessor := glob.GetAccessor(arg.Accessor, ctx)
	fd, err := accessor.Open(file)
	if err != nil {
		scope.Log("%s: %s", self.Name(), err.Error())
		return
	}
	defer fd.Close()

	buf := make([]byte, arg.Chunk)
	for {
		select {
		case <-ctx.Done():
			return

		default:
			n, err := fd.Read(buf)
			if err != nil || n == 0 {
				return
			}

			response := &ReadFileResponse{
				Data:     string(buf[:n]),
				Offset:   total_len,
				Filename: file,
			}
			output_chan <- response
			total_len += int64(n)
		}
		if arg.MaxLength > 0 &&
			total_len > int64(arg.MaxLength) {
			break
		}
	}

}

func (self ReadFilePlugin) Call(
	ctx context.Context,
	scope *vfilter.Scope,
	args *vfilter.Dict) <-chan vfilter.Row {
	output_chan := make(chan vfilter.Row)

	arg := &ReadFileArgs{}
	err := vfilter.ExtractArgs(scope, args, arg)
	if err != nil {
		scope.Log("%s: %s", self.Name(), err.Error())
		close(output_chan)
		return output_chan
	}

	if arg.Chunk == 0 {
		arg.Chunk = 4 * 1024 * 1024
	}

	go func() {
		defer close(output_chan)
		for _, file := range arg.Filenames {
			self.processFile(ctx, scope, arg, file, output_chan)
		}
	}()

	return output_chan
}

func (self ReadFilePlugin) Name() string {
	return "read_file"
}

func (self ReadFilePlugin) Info(scope *vfilter.Scope, type_map *vfilter.TypeMap) *vfilter.PluginInfo {
	return &vfilter.PluginInfo{
		Name:    "read_file",
		Doc:     "Read files in chunks.",
		RowType: type_map.AddType(scope, ReadFileResponse{}),
		ArgType: type_map.AddType(scope, &ReadFileArgs{}),
	}
}

type StatArgs struct {
	Filename []string `vfilter:"required,field=filename"`
	Accessor string   `vfilter:"optional,field=accessor"`
}

type StatPlugin struct{}

func (self *StatPlugin) Call(
	ctx context.Context,
	scope *vfilter.Scope,
	args *vfilter.Dict) <-chan vfilter.Row {
	output_chan := make(chan vfilter.Row)

	go func() {
		defer close(output_chan)

		arg := &StatArgs{}
		err := vfilter.ExtractArgs(scope, args, arg)
		if err != nil {
			scope.Log("%s: %s", "stat", err.Error())
			return
		}

		accessor := glob.GetAccessor(arg.Accessor, ctx)
		for _, filename := range arg.Filename {
			f, err := accessor.Lstat(filename)
			if err == nil {
				output_chan <- f
			}
		}
	}()

	return output_chan
}

func (self StatPlugin) Name() string {
	return "stat"
}

func (self StatPlugin) Info(scope *vfilter.Scope, type_map *vfilter.TypeMap) *vfilter.PluginInfo {
	return &vfilter.PluginInfo{
		Name:    "stat",
		Doc:     "Get file information. Unlike glob() this does not support wildcards.",
		ArgType: "StatArgs",
	}
}

func init() {
	vql_subsystem.RegisterPlugin(&GlobPlugin{})
	vql_subsystem.RegisterPlugin(&ReadFilePlugin{})
	vql_subsystem.RegisterPlugin(
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
		})
	vql_subsystem.RegisterPlugin(&StatPlugin{})
}
