package filesystem

import (
	"context"

	"github.com/Velocidex/ordereddict"
	"www.velocidex.com/golang/velociraptor/accessors"
	vql_subsystem "www.velocidex.com/golang/velociraptor/vql"
	vfilter "www.velocidex.com/golang/vfilter"
	"www.velocidex.com/golang/vfilter/arg_parser"
)

type ReadKeyValuesArgs struct {
	Globs    []string          `vfilter:"optional,field=globs,doc=Glob expressions to apply."`
	Accessor string            `vfilter:"optional,field=accessor,default=registry,doc=The accessor to use."`
	Root     *accessors.OSPath `vfilter:"optional,field=root,doc=The root directory to glob from (default '/')."`
}

type ReadKeyValues struct{}

// This is a convenience wrapper around the glob plugin. If the
// matching values is a directory (or registry key), this plugin will
// read its contents and assign each name as a key in the row dict.
func (self ReadKeyValues) Call(
	ctx context.Context,
	scope vfilter.Scope,
	args *ordereddict.Dict) <-chan vfilter.Row {
	output_chan := make(chan vfilter.Row)

	go func() {
		defer close(output_chan)
		defer vql_subsystem.RegisterMonitor(ctx, "read_reg_key", args)()

		arg := &ReadKeyValuesArgs{}
		err := arg_parser.ExtractArgsWithContext(ctx, scope, args, arg)
		if err != nil {
			scope.Log("read_reg_key: %s", err.Error())
			return
		}

		// By default use the registry accessor
		if arg.Accessor == "" {
			arg.Accessor = "registry"
			args.Set("accessor", "registry")
		}

		accessor, err := accessors.GetAccessor(arg.Accessor, scope)
		if err != nil {
			scope.Log("read_reg_key: %v", err)
			return
		}

		emit_dict := func(file_info accessors.FileInfo) {
			if !file_info.IsDir() {
				return
			}

			values, err := accessor.ReadDirWithOSPath(file_info.OSPath())
			if err != nil {
				return
			}

			result := ordereddict.NewDict().
				SetDefault(&vfilter.Null{}).
				SetCaseInsensitive().
				Set("Key", file_info)

			for _, item := range values {
				value_data := item.Data()
				if value_data != nil {
					value, pres := value_data.Get("value")
					if pres {
						name := item.Name()
						if name == "" {
							name = "@"
						}
						result.Set(name, value)
					}
				}
			}

			select {
			case <-ctx.Done():
				return

			case output_chan <- result:
			}
		}

		if arg.Root != nil && arg.Globs == nil {
			file_info, err := accessor.LstatWithOSPath(arg.Root)
			if err != nil {
				scope.Log("read_reg_key: %v: %v", arg.Root, err)
				return
			}
			emit_dict(file_info)
			return
		}

		// Delegate to the glob plugin.
		for row := range (GlobPlugin{}).Call(ctx, scope, args) {
			file_info, ok := row.(accessors.FileInfo)
			if ok {
				emit_dict(file_info)
			}
		}
	}()

	return output_chan
}

func (self ReadKeyValues) Info(scope vfilter.Scope, type_map *vfilter.TypeMap) *vfilter.PluginInfo {
	return &vfilter.PluginInfo{
		Name: "read_reg_key",
		Doc: "This is a convenience function for reading the entire " +
			"registry key matching the globs. Emits dicts with keys " +
			"being the value names and the values being the value data.",
		ArgType: type_map.AddType(scope, &ReadKeyValuesArgs{}),
	}
}

func init() {
	vql_subsystem.RegisterPlugin(&ReadKeyValues{})
}
