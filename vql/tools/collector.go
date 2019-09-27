package tools

import (
	"context"

	actions_proto "www.velocidex.com/golang/velociraptor/actions/proto"
	"www.velocidex.com/golang/velociraptor/artifacts"
	"www.velocidex.com/golang/velociraptor/config"
	"www.velocidex.com/golang/velociraptor/reporting"
	vql_subsystem "www.velocidex.com/golang/velociraptor/vql"
	"www.velocidex.com/golang/vfilter"
)

type CollectPluginArgs struct {
	Artifacts []string    `vfilter:"required,field=artifacts,doc=A list of artifacts to collect."`
	Output    string      `vfilter:"required,field=output,doc=A path to write the output file on."`
	Args      vfilter.Any `vfilter:"optional,field=args,doc=Optional parameters."`
}

type CollectPlugin struct{}

func (self CollectPlugin) Call(
	ctx context.Context,
	scope *vfilter.Scope,
	args *vfilter.Dict) <-chan vfilter.Row {
	output_chan := make(chan vfilter.Row)

	go func() {
		defer close(output_chan)

		arg := &CollectPluginArgs{}
		err := vfilter.ExtractArgs(scope, args, arg)
		if err != nil {
			scope.Log("collect: %v", err)
			return
		}

		config_obj := config.GetDefaultConfig()
		container, err := reporting.NewContainer(arg.Output)
		if err != nil {
			scope.Log("collect: %v", err)
			return
		}
		defer func() {
			container.Close()
			output_chan <- vfilter.NewDict().
				Set("Container", arg.Output)
		}()

		// Any uploads go into the container.
		subscope := scope.Copy()
		env := vfilter.NewDict().
			Set("$uploader", container)

		subscope.AppendVars(env)

		repository, err := artifacts.GetGlobalRepository(
			config_obj)
		for _, name := range arg.Artifacts {
			artifact, pres := repository.Get(name)
			if !pres {
				subscope.Log("collect: Unknown artifact %v", name)
				continue

			}

			request := &actions_proto.VQLCollectorArgs{}
			err := repository.Compile(artifact, request)
			if err != nil {
				subscope.Log("collect: Invalid artifact %v: %v",
					name, err)
				continue
			}

			// First set defaulst
			for _, e := range request.Env {
				env.Set(e.Key, e.Value)
			}

			in_params := func(key string) bool {
				for _, param := range artifact.Parameters {
					if param.Name == key {
						return true
					}
				}
				return false
			}

			// Now override provided parameters
			for _, key := range subscope.GetMembers(arg.Args) {
				if !in_params(key) {
					scope.Log("Unknown arg %v to artifact collector",
						key)
					return
				}

				value, pres := subscope.Associative(arg.Args,
					key)
				if pres {
					env.Set(key, value)
				}
			}

			for _, query := range request.Query {
				vql, err := vfilter.Parse(query.VQL)
				if err != nil {
					subscope.Log("collect: %v", err)
					return
				}

				err = container.StoreArtifact(
					config_obj,
					ctx, subscope, vql, query)
				if err != nil {
					subscope.Log("collect: %v", err)
					return
				}

				if query.Name != "" {
					subscope.Log("Collected %s", query.Name)
				}
			}
		}
	}()

	return output_chan
}

func (self CollectPlugin) Info(scope *vfilter.Scope, type_map *vfilter.TypeMap) *vfilter.PluginInfo {
	return &vfilter.PluginInfo{
		Name: "collect",
		Doc:  "Collect artifacts into a local file.",
		ArgType: type_map.AddType(scope,
			&CollectPluginArgs{}),
	}
}

func init() {
	vql_subsystem.RegisterPlugin(&CollectPlugin{})
}
