package server

import (
	"context"

	"github.com/Velocidex/ordereddict"
	"www.velocidex.com/golang/velociraptor/acls"
	"www.velocidex.com/golang/velociraptor/artifacts"
	artifacts_proto "www.velocidex.com/golang/velociraptor/artifacts/proto"
	flows_proto "www.velocidex.com/golang/velociraptor/flows/proto"
	"www.velocidex.com/golang/velociraptor/services"
	vql_subsystem "www.velocidex.com/golang/velociraptor/vql"
	"www.velocidex.com/golang/vfilter"
)

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
			artifact, pres := repository.Get(name)
			if pres {
				seen[artifact.Name] = artifact
			}
		}

		acl_manager := vql_subsystem.NullACLManager{}

		request, err := services.GetLauncher().CompileCollectorArgs(
			ctx, acl_manager, repository, &flows_proto.ArtifactCollectorArgs{
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
