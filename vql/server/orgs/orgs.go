package orgs

import (
	"context"

	"github.com/Velocidex/ordereddict"
	"github.com/Velocidex/yaml/v2"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	"www.velocidex.com/golang/velociraptor/services"
	vql_subsystem "www.velocidex.com/golang/velociraptor/vql"
	"www.velocidex.com/golang/vfilter"
)

type OrgsPlugin struct{}

func (self OrgsPlugin) Call(
	ctx context.Context,
	scope vfilter.Scope,
	args *ordereddict.Dict) <-chan vfilter.Row {
	output_chan := make(chan vfilter.Row)

	go func() {
		defer close(output_chan)
		defer vql_subsystem.RegisterMonitor(ctx, "orgs", args)()

		user_manager := services.GetUserManager()
		org_manager, err := services.GetOrgManager()
		if err != nil {
			scope.Log("orgs: %v", err)
			return
		}

		// ACLs are checked by the users module
		principal := vql_subsystem.GetPrincipal(scope)
		for _, org_record := range user_manager.GetOrgs(ctx, principal) {
			org_config_obj, err := org_manager.GetOrgConfig(org_record.Id)
			if err != nil {
				continue
			}

			client_config := &config_proto.Config{
				Version: org_config_obj.Version,
				Client:  org_config_obj.Client,
			}

			serialized, err := yaml.Marshal(client_config)
			if err != nil {
				continue
			}

			row := ordereddict.NewDict().
				Set("Name", org_record.Name).
				Set("OrgId", org_record.Id)

			if org_record.Nonce != "" {
				row.Set("_client_config", string(serialized))
			}

			select {
			case <-ctx.Done():
				return
			case output_chan <- row:
			}
		}

	}()

	return output_chan
}

func (self OrgsPlugin) Info(scope vfilter.Scope, type_map *vfilter.TypeMap) *vfilter.PluginInfo {
	return &vfilter.PluginInfo{
		Name: "orgs",
		Doc:  "Retrieve the list of orgs on this server.",
	}
}

func init() {
	vql_subsystem.RegisterPlugin(&OrgsPlugin{})
}
