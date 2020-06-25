package server

import (
	"context"

	"github.com/Velocidex/ordereddict"
	"www.velocidex.com/golang/velociraptor/acls"
	"www.velocidex.com/golang/velociraptor/artifacts"
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

		repository, err := artifacts.GetGlobalRepository(config_obj)
		if err != nil {
			scope.Log("artifact_definitions: %v", err)
			return
		}

		dependencies := make(map[string]int)
		for _, name := range arg.Names {
			dependencies[name] = 1

			artifact, pres := repository.Get(name)
			if !pres {
				scope.Log("Artifact %s not know", name)
				continue
			}

			for _, source := range artifact.Sources {
				err := repository.GetQueryDependencies(
					source.Query, 0, dependencies)
				if err != nil {
					scope.Log("artifact_definitions: %v", err)
					return
				}
			}
		}

		for k := range dependencies {
			artifact, pres := repository.Get(k)
			if pres {
				output_chan <- vfilter.RowToDict(ctx, scope, artifact)
			}
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
