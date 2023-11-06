package server

import (
	"context"
	"strings"

	"github.com/Velocidex/ordereddict"
	"google.golang.org/protobuf/proto"
	"www.velocidex.com/golang/velociraptor/acls"
	artifacts_proto "www.velocidex.com/golang/velociraptor/artifacts/proto"
	"www.velocidex.com/golang/velociraptor/json"
	"www.velocidex.com/golang/velociraptor/services"
	"www.velocidex.com/golang/velociraptor/vql"
	vql_subsystem "www.velocidex.com/golang/velociraptor/vql"
	"www.velocidex.com/golang/vfilter"
	"www.velocidex.com/golang/vfilter/arg_parser"
)

type ArtifactSetFunctionArgs struct {
	Definition string `vfilter:"optional,field=definition,doc=Artifact definition in YAML"`
	Prefix     string `vfilter:"optional,field=prefix,doc=Required name prefix"`
}

type ArtifactSetFunction struct{}

func (self *ArtifactSetFunction) Call(ctx context.Context,
	scope vfilter.Scope,
	args *ordereddict.Dict) vfilter.Any {

	arg := &ArtifactSetFunctionArgs{}
	err := arg_parser.ExtractArgsWithContext(ctx, scope, args, arg)
	if err != nil {
		scope.Log("artifact_set: %v", err)
		return vfilter.Null{}
	}

	config_obj, ok := vql_subsystem.GetServerConfig(scope)
	if !ok {
		scope.Log("artifact_set: Command can only run on the server")
		return vfilter.Null{}
	}

	manager, _ := services.GetRepositoryManager(config_obj)
	if manager == nil {
		scope.Log("artifact_set: Command can only run on the server")
		return vfilter.Null{}
	}

	tmp_repository := manager.NewRepository()
	definition, err := tmp_repository.LoadYaml(arg.Definition,
		services.ArtifactOptions{
			ValidateArtifact:  true,
			ArtifactIsBuiltIn: false,
		})
	if err != nil {
		definition := arg.Definition
		if len(arg.Definition) > 100 {
			definition = arg.Definition[:99] + " ..."
		}
		scope.Log("artifact_set: %v: %v", err, definition)
		return vfilter.Null{}
	}

	// Determine the permission required based on the type of the artifact.
	var permission acls.ACL_PERMISSION

	def_type := strings.ToLower(definition.Type)

	switch def_type {
	case "client", "client_event", "":
		permission = acls.ARTIFACT_WRITER
	case "server", "server_event":
		permission = acls.SERVER_ARTIFACT_WRITER
	default:
		scope.Log("artifact_set: artifact type %v invalid", definition.Type)
		return vfilter.Null{}
	}

	err = vql_subsystem.CheckAccess(scope, permission)
	if err != nil {
		scope.Log("artifact_set: %s", err)
		return vfilter.Null{}
	}

	principal := vql_subsystem.GetPrincipal(scope)

	definition, err = manager.SetArtifactFile(ctx,
		config_obj, principal, arg.Definition, arg.Prefix)
	if err != nil {
		scope.Log("artifact_set: %s", err)
		return vfilter.Null{}
	}

	return json.ConvertProtoToOrderedDict(definition)
}

func (self ArtifactSetFunction) Info(
	scope vfilter.Scope, type_map *vfilter.TypeMap) *vfilter.FunctionInfo {
	return &vfilter.FunctionInfo{
		Name:    "artifact_set",
		Doc:     "Sets an artifact into the global repository.",
		ArgType: type_map.AddType(scope, &ArtifactSetFunctionArgs{}),
		Metadata: vql.VQLMetadata().Permissions(
			acls.ARTIFACT_WRITER, acls.SERVER_ARTIFACT_WRITER).Build(),
	}
}

type ArtifactDeleteFunctionArgs struct {
	Name string `vfilter:"optional,field=name,doc=The Artifact to delete"`
}

type ArtifactDeleteFunction struct{}

func (self *ArtifactDeleteFunction) Call(ctx context.Context,
	scope vfilter.Scope,
	args *ordereddict.Dict) vfilter.Any {

	arg := &ArtifactDeleteFunctionArgs{}
	err := arg_parser.ExtractArgsWithContext(ctx, scope, args, arg)
	if err != nil {
		scope.Log("artifact_delete: %v", err)
		return vfilter.Null{}
	}

	config_obj, ok := vql_subsystem.GetServerConfig(scope)
	if !ok {
		scope.Log("artifact_delete: Command can only run on the server")
		return vfilter.Null{}
	}

	manager, _ := services.GetRepositoryManager(config_obj)
	if manager == nil {
		scope.Log("artifact_delete: Command can only run on the server")
		return vfilter.Null{}
	}

	global_repository, err := manager.GetGlobalRepository(config_obj)
	if err != nil {
		scope.Log("artifact_delete: %v", err)
		return vfilter.Null{}
	}

	definition, pres := global_repository.Get(ctx, config_obj, arg.Name)
	if !pres {
		scope.Log("artifact_delete: Artifact '%v' not found", arg.Name)
		return vfilter.Null{}
	}

	if definition.BuiltIn {
		scope.Log("artifact_delete: Can only delete custom artifacts.")
		return vfilter.Null{}
	}

	var permission acls.ACL_PERMISSION
	def_type := strings.ToLower(definition.Type)

	switch def_type {
	case "client", "client_event", "notebook", "":
		permission = acls.ARTIFACT_WRITER
	case "server", "server_event":
		permission = acls.SERVER_ARTIFACT_WRITER
	}

	err = vql_subsystem.CheckAccess(scope, permission)
	if err != nil {
		scope.Log("artifact_set: %s", err)
		return vfilter.Null{}
	}

	principal := vql_subsystem.GetPrincipal(scope)
	err = manager.DeleteArtifactFile(ctx, config_obj, principal, arg.Name)
	if err != nil {
		scope.Log("artifact_delete: %s", err)
		return vfilter.Null{}
	}

	return arg.Name
}

func (self ArtifactDeleteFunction) Info(
	scope vfilter.Scope, type_map *vfilter.TypeMap) *vfilter.FunctionInfo {
	return &vfilter.FunctionInfo{
		Name:    "artifact_delete",
		Doc:     "Deletes an artifact from the global repository.",
		ArgType: type_map.AddType(scope, &ArtifactDeleteFunctionArgs{}),
		Metadata: vql.VQLMetadata().Permissions(
			acls.ARTIFACT_WRITER, acls.SERVER_ARTIFACT_WRITER).Build(),
	}
}

type ArtifactsPluginArgs struct {
	Names               []string `vfilter:"optional,field=names,doc=Artifact definitions to dump"`
	IncludeDependencies bool     `vfilter:"optional,field=deps,doc=If true includes all dependencies as well."`
	Sanitize            bool     `vfilter:"optional,field=sanitize,doc=If true we remove extra metadata."`
}

type ArtifactsPlugin struct{}

func (self ArtifactsPlugin) Call(
	ctx context.Context,
	scope vfilter.Scope,
	args *ordereddict.Dict) <-chan vfilter.Row {
	output_chan := make(chan vfilter.Row)
	go func() {
		defer close(output_chan)

		err := vql_subsystem.CheckAccess(scope, acls.READ_RESULTS)
		if err != nil {
			scope.Log("artifact_definitions: %v", err)
			return
		}

		arg := &ArtifactsPluginArgs{}
		err = arg_parser.ExtractArgsWithContext(ctx, scope, args, arg)
		if err != nil {
			scope.Log("artifact_definitions: %v", err)
			return
		}

		config_obj, ok := vql_subsystem.GetServerConfig(scope)
		if !ok {
			scope.Log("artifact_definitions: Command can only run on the server")
			return
		}

		manager, err := services.GetRepositoryManager(config_obj)
		if err != nil {
			scope.Log("artifact_definitions: %v", err)
			return
		}
		repository, err := manager.GetGlobalRepository(config_obj)
		if err != nil {
			scope.Log("artifact_definitions: %v", err)
			return
		}

		// No args means just dump all artifacts
		if len(arg.Names) == 0 {
			names, err := repository.List(ctx, config_obj)
			if err != nil {
				scope.Log("artifact_definitions: %v", err)
				return
			}
			for _, name := range names {
				artifact, pres := repository.Get(ctx, config_obj, name)
				if !pres {
					continue
				}

				// Clean up the artifact by removing internal fields.
				artifact = proto.Clone(artifact).(*artifacts_proto.Artifact)
				for _, source := range artifact.Sources {
					if source.Query != "" && len(source.Queries) > 0 {
						source.Queries = nil
					}
				}

				if pres {
					select {
					case <-ctx.Done():
						return
					case output_chan <- json.ConvertProtoToOrderedDict(artifact):
					}
				}
			}
			return
		}

		seen := make(map[string]*artifacts_proto.Artifact)
		for _, name := range arg.Names {
			artifact, pres := repository.Get(ctx, config_obj, name)
			if !pres {
				scope.Log("artifact_definitions: artifact %v not known", name)
				continue
			}
			seen[artifact.Name] = artifact
		}

		launcher, err := services.GetLauncher(config_obj)
		if err != nil {
			scope.Log("artifact_definitions: Command can only run on the server %v", err)
			return
		}

		if arg.IncludeDependencies {
			deps, err := launcher.GetDependentArtifacts(ctx,
				config_obj, repository, arg.Names)
			if err != nil {
				scope.Log("artifact_definitions: %v", err)
				return
			}

			for _, name := range deps {
				if name == "" {
					continue
				}
				artifact, pres := repository.Get(ctx, config_obj, name)
				if pres {
					seen[artifact.Name] = artifact
				}
			}
		}

		for _, artifact := range seen {
			select {
			case <-ctx.Done():
				return
			case output_chan <- json.ConvertProtoToOrderedDict(artifact):
			}
		}
	}()

	return output_chan
}

func (self ArtifactsPlugin) Info(scope vfilter.Scope, type_map *vfilter.TypeMap) *vfilter.PluginInfo {
	return &vfilter.PluginInfo{
		Name:     "artifact_definitions",
		Doc:      "Dump artifact definitions.",
		ArgType:  type_map.AddType(scope, &ArtifactsPluginArgs{}),
		Metadata: vql.VQLMetadata().Permissions(acls.READ_RESULTS).Build(),
	}
}

type ArtifactSetMetadataFunctionArgs struct {
	Name   string `vfilter:"required,field=name,doc=The Artifact to update"`
	Hidden bool   `vfilter:"optional,field=hidden,doc=Set to true make the artifact hidden in the GUI, false to make it visible again."`
}

type ArtifactSetMetadataFunction struct{}

func (self *ArtifactSetMetadataFunction) Call(ctx context.Context,
	scope vfilter.Scope,
	args *ordereddict.Dict) vfilter.Any {

	arg := &ArtifactSetMetadataFunctionArgs{}
	err := arg_parser.ExtractArgsWithContext(ctx, scope, args, arg)
	if err != nil {
		scope.Log("artifact_set_metadata: %v", err)
		return vfilter.Null{}
	}

	config_obj, ok := vql_subsystem.GetServerConfig(scope)
	if !ok {
		scope.Log("artifact_set_metadata: Command can only run on the server")
		return vfilter.Null{}
	}

	manager, _ := services.GetRepositoryManager(config_obj)
	if manager == nil {
		scope.Log("artifact_set_metadata: Command can only run on the server")
		return vfilter.Null{}
	}

	global_repository, err := manager.GetGlobalRepository(config_obj)
	if err != nil {
		scope.Log("artifact_delete: %v", err)
		return vfilter.Null{}
	}

	definition, pres := global_repository.Get(ctx, config_obj, arg.Name)
	if !pres {
		scope.Log("artifact_delete: Artifact '%v' not found", arg.Name)
		return vfilter.Null{}
	}

	metadata := definition.Metadata
	if metadata == nil {
		metadata = &artifacts_proto.ArtifactMetadata{}
	}

	var permission acls.ACL_PERMISSION
	def_type := strings.ToLower(definition.Type)

	switch def_type {
	case "client", "client_event", "notebook", "":
		permission = acls.ARTIFACT_WRITER
	case "server", "server_event":
		permission = acls.SERVER_ARTIFACT_WRITER
	}

	err = vql_subsystem.CheckAccess(scope, permission)
	if err != nil {
		scope.Log("artifact_set_metadata: %s", err)
		return vfilter.Null{}
	}

	_, pres = args.Get("hidden")
	if pres {
		metadata.Hidden = arg.Hidden
	}

	principal := vql_subsystem.GetPrincipal(scope)
	err = manager.SetArtifactMetadata(ctx, config_obj, principal, arg.Name, metadata)
	if err != nil {
		scope.Log("artifact_set_metadata: %s", err)
		return vfilter.Null{}
	}

	return metadata
}

func (self ArtifactSetMetadataFunction) Info(
	scope vfilter.Scope, type_map *vfilter.TypeMap) *vfilter.FunctionInfo {
	return &vfilter.FunctionInfo{
		Name:    "artifact_set_metadata",
		Doc:     "Sets metadata about the artifact.",
		ArgType: type_map.AddType(scope, &ArtifactSetMetadataFunctionArgs{}),
		Metadata: vql.VQLMetadata().Permissions(
			acls.ARTIFACT_WRITER, acls.SERVER_ARTIFACT_WRITER).Build(),
	}
}

func init() {
	vql_subsystem.RegisterFunction(&ArtifactSetMetadataFunction{})
	vql_subsystem.RegisterPlugin(&ArtifactsPlugin{})
	vql_subsystem.RegisterFunction(&ArtifactSetFunction{})
	vql_subsystem.RegisterFunction(&ArtifactDeleteFunction{})
}
