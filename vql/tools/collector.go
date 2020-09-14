// +build server_vql

package tools

import (
	"context"
	"os"

	"github.com/Velocidex/ordereddict"
	"www.velocidex.com/golang/velociraptor/acls"
	"www.velocidex.com/golang/velociraptor/artifacts"
	artifacts_proto "www.velocidex.com/golang/velociraptor/artifacts/proto"
	"www.velocidex.com/golang/velociraptor/config"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	flows_proto "www.velocidex.com/golang/velociraptor/flows/proto"
	"www.velocidex.com/golang/velociraptor/json"
	"www.velocidex.com/golang/velociraptor/reporting"
	"www.velocidex.com/golang/velociraptor/services"
	vql_subsystem "www.velocidex.com/golang/velociraptor/vql"
	"www.velocidex.com/golang/vfilter"
)

type CollectPluginArgs struct {
	Artifacts           []string    `vfilter:"required,field=artifacts,doc=A list of artifacts to collect."`
	Output              string      `vfilter:"required,field=output,doc=A path to write the output file on."`
	Report              string      `vfilter:"optional,field=report,doc=A path to write the report on."`
	Args                vfilter.Any `vfilter:"optional,field=args,doc=Optional parameters."`
	Password            string      `vfilter:"optional,field=password,doc=An optional password to encrypt the collection zip."`
	Format              string      `vfilter:"optional,field=format,doc=Output format (csv, jsonl)."`
	ArtifactDefinitions vfilter.Any `vfilter:"optional,field=artifact_definitions,doc=Optional additional custom artifacts."`
	Template            string      `vfilter:"optional,field=template,doc=The name of a template artifact (i.e. one which has report of type HTML)."`
}

type CollectPlugin struct{}

func (self CollectPlugin) Call(
	ctx context.Context,
	scope *vfilter.Scope,
	args *ordereddict.Dict) <-chan vfilter.Row {
	output_chan := make(chan vfilter.Row)

	go func() {
		defer close(output_chan)

		var container *reporting.Container

		// This plugin allows one to create files, collect
		// artifacts and also define new artifacts. It is very
		// privileged.
		err := vql_subsystem.CheckAccess(scope,
			acls.COLLECT_SERVER, acls.ARTIFACT_WRITER,
			acls.FILESYSTEM_WRITE,
			acls.SERVER_ARTIFACT_WRITER)
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

		if arg.Template == "" {
			arg.Template = "Reporting.Default"
		}

		config_obj, ok := artifacts.GetServerConfig(scope)
		if !ok {
			config_obj = config.GetDefaultConfig()
		}
		repository, err := getRepository(config_obj, arg.ArtifactDefinitions)
		if err != nil {
			scope.Log("collect: %v", err)
			return
		}

		artifact_definitions := []*artifacts_proto.Artifact{}
		definitions := []*artifacts_proto.Artifact{}
		for _, name := range arg.Artifacts {
			artifact, pres := repository.Get(config_obj, name)
			if !pres {
				scope.Log("Artifact %v not known.", name)
				return
			}
			definitions = append(definitions, artifact)
		}

		if arg.Output != "" {
			container, err = reporting.NewContainer(arg.Output)
			if err != nil {
				scope.Log("collect: %v", err)
				return
			}

			scope.Log("Will create container at %s", arg.Output)

			// On exit we create a report.
			defer func() {
				container.Close()

				if arg.Report != "" {
					fd, err := os.OpenFile(
						arg.Report, os.O_RDWR|os.O_CREATE|os.O_TRUNC, 0700)
					if err != nil {
						scope.Log("Error creating report: %v", err)
						return
					}
					defer fd.Close()

					err = produceReport(config_obj, container,
						arg.Template,
						repository, fd,
						artifact_definitions,
						scope, arg)
					if err != nil {
						scope.Log("Error creating report: %v", err)
					}
				}
				output_chan <- ordereddict.NewDict().
					Set("Container", arg.Output).
					Set("Report", arg.Report)
			}()

			// Should we encrypt it?
			if arg.Password != "" {
				container.Password = arg.Password
				scope.Log("Will password protect container")
			}
		}

		builder := services.ScopeBuilderFromScope(scope)
		if container != nil {
			builder.Uploader = container
		}

		for _, name := range arg.Artifacts {
			artifact, pres := repository.Get(config_obj, name)
			if !pres {
				scope.Log("collect: Unknown artifact %v", name)
				continue

			}

			launcher, err := services.GetLauncher()
			if err != nil {
				scope.Log("collect: %v", err)
				return
			}

			err = launcher.EnsureToolsDeclared(
				ctx, config_obj, artifact)
			if err != nil {
				scope.Log("collect: %v %v", name, err)
				continue
			}

			artifact_definitions = append(artifact_definitions, artifact)
			acl_manager, ok := artifacts.GetACLManager(scope)
			if !ok {
				acl_manager = vql_subsystem.NullACLManager{}
			}

			request, err := launcher.CompileCollectorArgs(
				ctx, config_obj, acl_manager, repository,
				&flows_proto.ArtifactCollectorArgs{
					Artifacts: []string{artifact.Name},
				})
			if err != nil {
				scope.Log("collect: Invalid artifact %v: %v",
					name, err)
				continue
			}

			// First set defaults
			builder.Env = ordereddict.NewDict()
			for _, e := range request.Env {
				builder.Env.Set(e.Key, e.Value)
			}

			// Now override provided parameters
			for _, key := range scope.GetMembers(arg.Args) {
				if !valid_parameter(key, definitions) {
					scope.Log("Unknown parameter %s - ignoring", key)
				}

				value, pres := scope.Associative(arg.Args, key)
				if pres {
					builder.Env.Set(key, value)
				}
			}

			// Make a new scope for each artifact.
			// Any uploads go into the container.
			manager, err := services.GetRepositoryManager()
			if err != nil {
				scope.Log("collect: %v %v", name, err)
				return
			}

			subscope := manager.BuildScope(builder)
			defer subscope.Close()

			for _, query := range request.Query {
				vql, err := vfilter.Parse(query.VQL)
				if err != nil {
					subscope.Log("collect: %v", err)
					return
				}

				// Dont store un-named queries but run them anyway.
				if query.Name == "" {
					for range vql.Eval(ctx, subscope) {
					}
					continue
				} else {
					subscope.Log("Starting collection of %s", query.Name)
				}

				// If no container is specified, just
				// push the result to the output
				// channel.
				if container == nil {
					for row := range vql.Eval(ctx, subscope) {
						output_chan <- row
					}
					continue
				}

				// Otherwise push the results into the container.
				err = container.StoreArtifact(
					config_obj, ctx, subscope, vql, query, arg.Format)
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

func getRepository(
	config_obj *config_proto.Config,
	extra_artifacts vfilter.Any) (services.Repository, error) {
	manager, err := services.GetRepositoryManager()
	if err != nil {
		return nil, err
	}
	repository, err := manager.GetGlobalRepository(config_obj)
	if err != nil {
		return nil, err
	}

	if extra_artifacts == nil {
		return repository, nil
	}

	// Private copy of the repository.
	repository = repository.Copy()

	loader := func(item *ordereddict.Dict) error {
		serialized, err := json.Marshal(item)
		if err != nil {
			return err
		}

		_, err = repository.LoadYaml(string(serialized), true /* validate */)
		if err != nil {
			return err
		}
		return nil
	}

	switch t := extra_artifacts.(type) {
	case []*ordereddict.Dict:
		for _, item := range t {
			err := loader(item)
			if err != nil {
				return nil, err
			}
		}

	case *ordereddict.Dict:
		err := loader(t)
		if err != nil {
			return nil, err
		}

	case []string:
		for _, item := range t {
			_, err := repository.LoadYaml(item, true /* validate */)
			if err != nil {
				return nil, err
			}
		}

	case string:
		_, err := repository.LoadYaml(t, true /* validate */)
		if err != nil {
			return nil, err
		}
	}

	return repository, nil
}

func (self CollectPlugin) Info(scope *vfilter.Scope, type_map *vfilter.TypeMap) *vfilter.PluginInfo {
	return &vfilter.PluginInfo{
		Name: "collect",
		Doc:  "Collect artifacts into a local file.",
		ArgType: type_map.AddType(scope,
			&CollectPluginArgs{}),
	}
}

// Check if the user specified an unknown parameter.
func valid_parameter(param_name string, definitions []*artifacts_proto.Artifact) bool {
	for _, definition := range definitions {
		for _, param := range definition.Parameters {
			if param.Name == param_name {
				return true
			}
		}
	}
	return false
}

func init() {
	vql_subsystem.RegisterPlugin(&CollectPlugin{})
}
