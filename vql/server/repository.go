package server

import (
	"context"
	"strings"

	"github.com/Velocidex/ordereddict"
	"www.velocidex.com/golang/velociraptor/acls"
	"www.velocidex.com/golang/velociraptor/artifacts"
	artifacts_proto "www.velocidex.com/golang/velociraptor/artifacts/proto"
	flows_proto "www.velocidex.com/golang/velociraptor/flows/proto"
	"www.velocidex.com/golang/velociraptor/services"
	vql_subsystem "www.velocidex.com/golang/velociraptor/vql"
	"www.velocidex.com/golang/vfilter"
)

type ArtifactSetFunctionArgs struct {
	Definition string `vfilter:"optional,field=definition,doc=Artifact definition in YAML"`
	Prefix     string `vfilter:"optional,field=prefix,doc=Required name prefix"`
}

type ArtifactSetFunction struct{}

func (self *ArtifactSetFunction) Call(ctx context.Context,
	scope *vfilter.Scope,
	args *ordereddict.Dict) vfilter.Any {

	arg := &ArtifactSetFunctionArgs{}
	err := vfilter.ExtractArgs(scope, args, arg)
	if err != nil {
		scope.Log("artifact_set: %v", err)
		return vfilter.Null{}
	}

	if arg.Prefix == "" {
		arg.Prefix = "Packs."
	}

	config_obj, ok := artifacts.GetServerConfig(scope)
	if !ok {
		scope.Log("artifact_set: Command can only run on the server")
		return vfilter.Null{}
	}

	manager := services.GetRepositoryManager()
	if manager == nil {
		scope.Log("artifact_set: Command can only run on the server")
		return vfilter.Null{}
	}

	tmp_repository := manager.NewRepository()
	definition, err := tmp_repository.LoadYaml(arg.Definition, true /* validate */)
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

	definition, err = manager.SetArtifactFile(config_obj, arg.Definition, arg.Prefix)
	if err != nil {
		scope.Log("artifact_set: %s", err)
		return vfilter.Null{}
	}

	return definition
}

func (self ArtifactSetFunction) Info(
	scope *vfilter.Scope, type_map *vfilter.TypeMap) *vfilter.FunctionInfo {
	return &vfilter.FunctionInfo{
		Name:    "artifact_set",
		Doc:     "Sets and artifact into the global repository.",
		ArgType: type_map.AddType(scope, &ArtifactSetFunctionArgs{}),
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
	scope *vfilter.Scope,
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
		err = vfilter.ExtractArgs(scope, args, arg)
		if err != nil {
			scope.Log("artifact_definitions: %v", err)
			return
		}

		config_obj, ok := artifacts.GetServerConfig(scope)
		if !ok {
			scope.Log("Command can only run on the server")
			return
		}

		repository, err := services.GetRepositoryManager().
			GetGlobalRepository(config_obj)
		if err != nil {
			scope.Log("artifact_definitions: %v", err)
			return
		}

		// No args means just dump all artifacts
		if len(arg.Names) == 0 {
			arg.Names = repository.List()
		}

		seen := make(map[string]*artifacts_proto.Artifact)
		for _, name := range arg.Names {
			artifact, pres := repository.Get(config_obj, name)
			if pres {
				seen[artifact.Name] = artifact
			}
		}

		acl_manager := vql_subsystem.NullACLManager{}

		request, err := services.GetLauncher().CompileCollectorArgs(
			ctx, config_obj, acl_manager,
			repository, &flows_proto.ArtifactCollectorArgs{
				Artifacts: arg.Names,
			})
		if err != nil {
			scope.Log("artifact_definitions: %v", err)
			return
		}

		for _, artifact := range request.Artifacts {
			seen[artifact.Name] = artifact
		}

		for _, artifact := range seen {
			output_chan <- vfilter.RowToDict(ctx, scope, artifact)
		}
	}()

	return output_chan
}

func (self ArtifactsPlugin) Info(scope *vfilter.Scope, type_map *vfilter.TypeMap) *vfilter.PluginInfo {
	return &vfilter.PluginInfo{
		Name:    "artifact_definitions",
		Doc:     "Dump artifact definitions.",
		ArgType: type_map.AddType(scope, &ArtifactsPluginArgs{}),
	}
}

func init() {
	vql_subsystem.RegisterPlugin(&ArtifactsPlugin{})
}
