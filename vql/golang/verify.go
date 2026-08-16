package golang

import (
	"context"

	"github.com/Velocidex/ordereddict"
	"www.velocidex.com/golang/velociraptor/services"
	"www.velocidex.com/golang/velociraptor/services/launcher"
	vql_subsystem "www.velocidex.com/golang/velociraptor/vql"
	vql_server "www.velocidex.com/golang/velociraptor/vql/server"
	"www.velocidex.com/golang/vfilter"
	"www.velocidex.com/golang/vfilter/arg_parser"
)

type VerifyFunctionArgs struct {
	Artifact        string `vfilter:"required,field=artifact,doc=The artifact to verify. This can be an artifact source in yaml or json or the name of an artifact"`
	Repository      string `vfilter:"optional,field=repository,doc=The repository to use for verification, if not set, we default to the global repository."`
	DisableOverride bool   `vfilter:"optional,field=disable_override,doc=If set, we do not allow override of built-in artifacts (allowed by default)"`
}

type VerifyFunction struct{}

func (self VerifyFunction) Call(ctx context.Context,
	scope vfilter.Scope,
	args *ordereddict.Dict) vfilter.Any {

	defer vql_subsystem.RegisterMonitor(ctx, "verify", args)()

	arg := &VerifyFunctionArgs{}
	err := arg_parser.ExtractArgsWithContext(ctx, scope, args, arg)
	if err != nil {
		scope.Log("verify: %v", err)
		return vfilter.Null{}
	}

	config_obj, ok := vql_subsystem.GetServerConfig(scope)
	if !ok {
		scope.Log("verify: Must run on the server")
		return vfilter.Null{}
	}

	manager, err := services.GetRepositoryManager(config_obj)
	if err != nil {
		scope.Log("verify: %v", err)
		return vfilter.Null{}
	}

	repository, err := manager.GetGlobalRepository(config_obj)
	if err != nil {
		scope.Log("verify: %v", err)
		return vfilter.Null{}
	}

	state := launcher.NewAnalysisState(arg.Artifact)

	if arg.Repository != "" {
		cached_any := vql_subsystem.CacheGet(
			scope, vql_server.REPOSITORY_CACHE_TAG+arg.Repository)

		cached_repository, ok := cached_any.(services.Repository)
		if ok {
			repository = cached_repository
		} else {
			// Make a local copy.
			repository = repository.Copy()

			// Cache for next time.
			vql_subsystem.CacheSet(scope,
				vql_server.REPOSITORY_CACHE_TAG+arg.Repository, repository)
		}

	} else {
		// Operate on a copy of the global repository
		repository = repository.Copy()
	}

	artifact, pres := repository.Get(ctx, config_obj, arg.Artifact)
	if !pres {
		artifact, err = repository.LoadYaml(arg.Artifact,
			services.ArtifactOptions{
				ValidateArtifact:     true,
				ArtifactIsBuiltIn:    !arg.DisableOverride,
				AllowOverridingAlias: true,
			})
		if err != nil {
			state.SetError(launcher.YAML_ERROR, launcher.YAML_ERROR_MSG, err)
			return stateToDict(state)
		}
	}

	// Verify the artifact
	launcher.VerifyArtifact(
		ctx, config_obj, repository, artifact, state)

	return stateToDict(state)
}

func stateToDict(state *launcher.AnalysisState) *ordereddict.Dict {
	var errors []string
	for _, e := range state.Errors {
		errors = append(errors, e.Error())
	}

	definitions := make(map[string]*ordereddict.Dict)
	for key, d := range state.Definitions {
		var args []string
		for _, arg := range d.Args {
			args = append(args, arg.Name)
		}

		definitions[key] = ordereddict.NewDict().
			Set("Type", d.Type).
			Set("Name", d.Name).
			Set("Args", args).
			Set("Defaults", d.Defaults)
	}

	return ordereddict.NewDict().
		Set("Artifact", state.Artifact).
		Set("Permissions", state.Permissions).
		Set("Errors", errors).
		Set("Warnings", state.Warnings).
		Set("Definitions", definitions).
		Set("Suppressions", state.Suppressions)
}

func (self VerifyFunction) Info(scope vfilter.Scope, type_map *vfilter.TypeMap) *vfilter.FunctionInfo {
	return &vfilter.FunctionInfo{
		Name: "verify",
		Doc: `verify an artifact

This function will verify the artifact and flag any potential errors or warnings.
`,
		Metadata: vql_subsystem.VQLMetadata().Build(),
		Version:  2,
		ArgType:  type_map.AddType(scope, &VerifyFunctionArgs{}),
	}
}

func init() {
	vql_subsystem.RegisterFunction(&VerifyFunction{})
}
