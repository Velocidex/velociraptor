package filesystem

import (
	"context"

	"github.com/Velocidex/ordereddict"
	"www.velocidex.com/golang/velociraptor/accessors"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	"www.velocidex.com/golang/velociraptor/json"
	vql_subsystem "www.velocidex.com/golang/velociraptor/vql"
	vfilter "www.velocidex.com/golang/vfilter"
	"www.velocidex.com/golang/vfilter/arg_parser"
)

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
		root := ""
		for _, item := range arg.Globs {
			item_root, item_path, _ := accessor.GetRoot(item)
			if root != "" && root != item_root {
				scope.Log("glob: %s: Must use the same root for "+
					"all globs. Skipping.", item)
				continue
			}
			root = item_root
			err = globber.Add(item_path, accessor.PathSplit)
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
						value_info, ok := item.(glob.FileInfo)
						if ok {
							value_data, ok := value_info.Data().(*ordereddict.Dict)
							if ok && value_data != nil {
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

	json.RegisterCustomEncoder(&RawRegKeyInfo{}, glob.MarshalGlobFileInfo)
	json.RegisterCustomEncoder(&RawRegValueInfo{}, glob.MarshalGlobFileInfo)
}
