package secrets

import (
	"context"

	"github.com/Velocidex/ordereddict"
	"www.velocidex.com/golang/velociraptor/acls"
	"www.velocidex.com/golang/velociraptor/json"
	"www.velocidex.com/golang/velociraptor/services"
	vql_subsystem "www.velocidex.com/golang/velociraptor/vql"
	"www.velocidex.com/golang/vfilter"
	"www.velocidex.com/golang/vfilter/arg_parser"
)

type SecretsPluginArgs struct {
}

type SecretsPlugin struct{}

func (self SecretsPlugin) Call(
	ctx context.Context,
	scope vfilter.Scope,
	args *ordereddict.Dict) <-chan vfilter.Row {
	output_chan := make(chan vfilter.Row)

	go func() {
		defer close(output_chan)
		defer vql_subsystem.RegisterMonitor(ctx, "secrets", args)()

		err := vql_subsystem.CheckAccess(scope, acls.SERVER_ADMIN)
		if err != nil {
			scope.Log("secrets: %v", err)
			return
		}

		arg := &SecretsPluginArgs{}
		err = arg_parser.ExtractArgsWithContext(ctx, scope, args, arg)
		if err != nil {
			scope.Log("secrets: %v", err)
			return
		}

		err = services.RequireFrontend()
		if err != nil {
			scope.Log("secrets: %v", err)
			return
		}

		org_config_obj, ok := vql_subsystem.GetServerConfig(scope)
		if !ok {
		}

		secrets, err := services.GetSecretsService(org_config_obj)
		if err != nil {
			scope.Log("secrets: Command can only run on the server")
			return
		}

		for _, details := range secrets.GetSecretDefinitions(ctx) {
			row := json.ConvertProtoToOrderedDict(details)
			type_name, _ := row.GetString("type_name")

			var secret_instances []*ordereddict.Dict

			secret_names, _ := row.GetStrings("secret_names")

			for _, s := range secret_names {
				secret, err := secrets.GetSecretMetadata(ctx, type_name, s)
				if err != nil {
					continue
				}
				secret_instances = append(secret_instances,
					json.ConvertProtoToOrderedDict(secret))
			}

			row.Delete("secret_names")
			row.Set("secrets", secret_instances)

			select {
			case <-ctx.Done():
				return
			case output_chan <- row:
			}
		}
	}()

	return output_chan
}

func (self SecretsPlugin) Info(scope vfilter.Scope, type_map *vfilter.TypeMap) *vfilter.PluginInfo {
	return &vfilter.PluginInfo{
		Name:    "secrets",
		Doc:     "Retrieve the list of secrets on the server.",
		ArgType: type_map.AddType(scope, &SecretsPluginArgs{}),
	}
}

func init() {
	vql_subsystem.RegisterPlugin(&SecretsPlugin{})
}
