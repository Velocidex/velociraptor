package golang

import (
	"context"

	"github.com/Velocidex/ordereddict"
	"www.velocidex.com/golang/velociraptor/services"
	"www.velocidex.com/golang/velociraptor/services/launcher"
	vql_subsystem "www.velocidex.com/golang/velociraptor/vql"
	"www.velocidex.com/golang/vfilter"
	"www.velocidex.com/golang/vfilter/arg_parser"
)

type VerifyFunctionArgs struct {
	Artifact string `vfilter:"required,field=artifact,doc=The artifact to verify. This can be an artifact source in yaml or json or the name of an artifact"`
}

func init() {
	vql_subsystem.RegisterFunction(
		vfilter.GenericFunction{
			FunctionName: "verify",
			Doc: `verify an artifact

This function will verify the artifact and flag any potential errors or warnings.
`,
			Metadata: vql_subsystem.VQLMetadata().Build(),
			ArgType:  &VerifyFunctionArgs{},
			Function: func(
				ctx context.Context,
				scope vfilter.Scope,
				args *ordereddict.Dict) vfilter.Any {

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
					return err
				}

				repository, err := manager.GetGlobalRepository(config_obj)
				if err != nil {
					return err
				}

				state := launcher.NewAnalysisState(arg.Artifact)

				artifact, pres := repository.Get(ctx, config_obj, arg.Artifact)
				if !pres {
					local_repository := manager.NewRepository()
					local_repository.SetParent(repository, config_obj)

					artifact, err = local_repository.LoadYaml(arg.Artifact,
						services.ArtifactOptions{
							ValidateArtifact:     true,
							ArtifactIsBuiltIn:    true,
							AllowOverridingAlias: true,
						})
					if err != nil {
						state.SetError(err)
						return state
					}

					repository = local_repository
				}

				// Verify the artifact
				launcher.VerifyArtifact(
					ctx, config_obj, repository, artifact, state)

				return state
			},
		})
}
