package tools

import (
	"context"

	"github.com/Velocidex/ordereddict"
	"www.velocidex.com/golang/velociraptor/acls"
	actions_proto "www.velocidex.com/golang/velociraptor/actions/proto"
	"www.velocidex.com/golang/velociraptor/artifacts"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	"www.velocidex.com/golang/velociraptor/reporting"
	vql_subsystem "www.velocidex.com/golang/velociraptor/vql"
	"www.velocidex.com/golang/vfilter"
)

type CollectPluginArgs struct {
	Artifacts []string    `vfilter:"required,field=artifacts,doc=A list of artifacts to collect."`
	Output    string      `vfilter:"required,field=output,doc=A path to write the output file on."`
	Args      vfilter.Any `vfilter:"optional,field=args,doc=Optional parameters."`
	Password  string      `vfilter:"optional,field=password,doc=An optional password to encrypt the collection zip."`
	Format    string      `vfilter:"optional,field=format,doc=Output format (csv, jsonl)."`
}

type CollectPlugin struct{}

func (self CollectPlugin) Call(
	ctx context.Context,
	scope *vfilter.Scope,
	args *ordereddict.Dict) <-chan vfilter.Row {
	output_chan := make(chan vfilter.Row)

	go func() {
		defer close(output_chan)

		// ACLs will be carried through to the collected
		// artifacts from this plugin.
		err := vql_subsystem.CheckAccess(scope, acls.COLLECT_CLIENT)
		if err != nil {
			scope.Log("collect: %s", err)
			return
		}

		arg := &CollectPluginArgs{}
		err = vfilter.ExtractArgs(scope, args, arg)
		if err != nil {
			scope.Log("collect: %v", err)
			return
		}

		switch arg.Format {
		case "jsonl", "csv", "json":
		case "":
			arg.Format = "jsonl"
		default:
			scope.Log("collect: format %v not supported", arg.Format)
			return
		}

		config_obj := &config_proto.Config{}
		container, err := reporting.NewContainer(arg.Output)
		if err != nil {
			scope.Log("collect: %v", err)
			return
		}
		defer func() {
			container.Close()
			output_chan <- ordereddict.NewDict().
				Set("Container", arg.Output)
		}()

		// Should we encrypt it?
		container.Password = arg.Password

		repository, err := artifacts.GetGlobalRepository(config_obj)
		if err != nil {
			scope.Log("collect: %v", err)
			return
		}

		builder := artifacts.ScopeBuilderFromScope(scope)
		builder.Uploader = container

		for _, name := range arg.Artifacts {
			artifact, pres := repository.Get(name)
			if !pres {
				scope.Log("collect: Unknown artifact %v", name)
				continue

			}

			request := &actions_proto.VQLCollectorArgs{}
			err := repository.Compile(artifact, request)
			if err != nil {
				scope.Log("collect: Invalid artifact %v: %v",
					name, err)
				continue
			}

			// First set defaulst
			builder.Env = ordereddict.NewDict()
			for _, e := range request.Env {
				builder.Env.Set(e.Key, e.Value)
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
			for _, key := range scope.GetMembers(arg.Args) {
				if !in_params(key) {
					scope.Log("Unknown arg %v to artifact collector",
						key)
					return
				}

				value, pres := scope.Associative(arg.Args,
					key)
				if pres {
					builder.Env.Set(key, value)
				}
			}

			// Make a new scope for each artifact.
			// Any uploads go into the container.
			subscope := builder.Build()

			for _, query := range request.Query {
				vql, err := vfilter.Parse(query.VQL)
				if err != nil {
					subscope.Log("collect: %v", err)
					return
				}

				err = container.StoreArtifact(
					config_obj,
					ctx, subscope, vql, query, arg.Format)
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
