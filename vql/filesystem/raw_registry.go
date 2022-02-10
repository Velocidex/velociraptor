package filesystem

import (
	"context"

	"github.com/Velocidex/ordereddict"
	"www.velocidex.com/golang/velociraptor/accessors"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	"www.velocidex.com/golang/velociraptor/glob"
	vql_subsystem "www.velocidex.com/golang/velociraptor/vql"
	vfilter "www.velocidex.com/golang/vfilter"
	"www.velocidex.com/golang/vfilter/arg_parser"
)

type ReadKeyValuesArgs struct {
	Globs    []string `vfilter:"required,field=globs,doc=Glob expressions to apply."`
	Accessor string   `vfilter:"optional,field=accessor,default=reg,doc=The accessor to use."`
	Root     string   `vfilter:"optional,field=root,doc=The root directory to glob from (default '/')."`
}

type ReadKeyValues struct{}

func (self ReadKeyValues) Call(
	ctx context.Context,
	scope vfilter.Scope,
	args *ordereddict.Dict) <-chan vfilter.Row {
	globber := glob.NewGlobber()
	output_chan := make(chan vfilter.Row)

	go func() {
		defer close(output_chan)

		config_obj, ok := vql_subsystem.GetServerConfig(scope)
		if !ok {
			config_obj = &config_proto.Config{}
		}

		arg := &ReadKeyValuesArgs{}
		err := arg_parser.ExtractArgsWithContext(ctx, scope, args, arg)
		if err != nil {
			scope.Log("read_reg_key: %s", err.Error())
			return
		}

		accessor_name := arg.Accessor
		if accessor_name == "" {
			accessor_name = "reg"
		}

		err = vql_subsystem.CheckFilesystemAccess(scope, arg.Accessor)
		if err != nil {
			scope.Log("read_reg_key: %s", err.Error())
			return
		}

		accessor, err := accessors.GetAccessor(accessor_name, scope)
		if err != nil {
			scope.Log("read_reg_key: %v", err)
			return
		}

		globs := glob.ExpandBraces(arg.Globs)
		root := accessor.ParsePath(arg.Root)
		for _, item := range globs {
			err = globber.Add(root.Parse(item))
			if err != nil {
				scope.Log("glob: %v", err)
				return
			}
		}

		file_chan := globber.ExpandWithContext(
			ctx, config_obj, root, accessor)
		for {
			select {
			case <-ctx.Done():
				return

			case f, ok := <-file_chan:
				if !ok {
					return
				}
				if f.IsDir() {
					res := ordereddict.NewDict().
						SetDefault(&vfilter.Null{}).
						SetCaseInsensitive().
						Set("Key", f)
					values, err := accessor.ReadDir(f.FullPath())
					if err != nil {
						continue
					}

					for _, item := range values {
						value_info, ok := item.(accessors.FileInfo)
						if ok {
							value_data := value_info.Data()
							if value_data != nil {
								value, pres := value_data.Get("value")
								if pres {
									res.Set(item.Name(), value)
								}
							}
						}
					}

					select {
					case <-ctx.Done():
						return

					case output_chan <- res:
					}
				}
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
