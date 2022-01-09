package server

import (
	"context"
	"strings"

	"github.com/Velocidex/ordereddict"
	"www.velocidex.com/golang/velociraptor/acls"
	artifacts_proto "www.velocidex.com/golang/velociraptor/artifacts/proto"
	"www.velocidex.com/golang/velociraptor/json"
	"www.velocidex.com/golang/velociraptor/services"
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

	manager, _ := services.GetRepositoryManager()
	if manager == nil {
		scope.Log("artifact_set: Command can only run on the server")
		return vfilter.Null{}
	}

	tmp_repository := manager.NewRepository()
	definition, err := tmp_repository.LoadYaml(
		arg.Definition, true /* validate */, false /* built_in */)
	if err != nil {
		scope.Log("artifact_set: %v", err)
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
		scope.Log("artifact_set: artifact type invalid")
		return vfilter.Null{}
	}

	err = vql_subsystem.CheckAccess(scope, permission)
	if err != nil {
		scope.Log("artifact_set: %s", err)
		return vfilter.Null{}
	}

	principal := vql_subsystem.GetPrincipal(scope)

	definition, err = manager.SetArtifactFile(
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

	manager, _ := services.GetRepositoryManager()
	if manager == nil {
		scope.Log("artifact_delete: Command can only run on the server")
		return vfilter.Null{}
	}

	global_repository, err := manager.GetGlobalRepository(config_obj)
	if err != nil {
		scope.Log("artifact_delete: %v", err)
		return vfilter.Null{}
	}

	definition, pres := global_repository.Get(config_obj, arg.Name)
	if !pres {
		scope.Log("artifact_delete: Artifact '%v' not found", arg.Name)
		return vfilter.Null{}
	}

	// Same criteria as
	// https://github.com/Velocidex/velociraptor/blob/3eddb529a0059a05c0a6c2c7057446f36c4e9a6a/gui/static/angular-components/artifact/artifact-viewer-directive.js#L62
	if !strings.HasPrefix(definition.Name, "Custom.") {
		scope.Log("artifact_delete: Can only delete custom artifacts.")
		return vfilter.Null{}
	}

	var permission acls.ACL_PERMISSION
	def_type := strings.ToLower(definition.Type)

	switch def_type {
	case "client", "client_event", "":
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
	err = manager.DeleteArtifactFile(config_obj, principal, arg.Name)
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
			scope.Log("Command can only run on the server")
			return
		}

		manager, err := services.GetRepositoryManager()
		if err != nil {
			scope.Log("Command can only run on the server")
			return
		}
		repository, err := manager.GetGlobalRepository(config_obj)
		if err != nil {
			scope.Log("artifact_definitions: %v", err)
			return
		}

		// No args means just dump all artifacts
		if len(arg.Names) == 0 {
			for _, name := range repository.List() {
				artifact, pres := repository.Get(config_obj, name)
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
			artifact, pres := repository.Get(config_obj, name)
			if pres {
				seen[artifact.Name] = artifact
			}
		}

		launcher, err := services.GetLauncher()
		if err != nil {
			scope.Log("artifact_definitions: %v", err)
			return
		}

		deps, err := launcher.GetDependentArtifacts(
			config_obj, repository, arg.Names)
		if err != nil {
			scope.Log("artifact_definitions: %v", err)
			return
		}

		for _, name := range deps {
			if name == "" {
				continue
			}
			artifact, pres := repository.Get(config_obj, name)
			if !pres {
				scope.Log("artifact_definitions: artifact %v not known", name)
				continue
			}

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
		Name:    "artifact_definitions",
		Doc:     "Dump artifact definitions.",
		ArgType: type_map.AddType(scope, &ArtifactsPluginArgs{}),
	}
}

func init() {
	vql_subsystem.RegisterPlugin(&ArtifactsPlugin{})
	vql_subsystem.RegisterFunction(&ArtifactSetFunction{})
	vql_subsystem.RegisterFunction(&ArtifactDeleteFunction{})
}
