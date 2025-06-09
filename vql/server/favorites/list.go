package favorites

import (
	"context"

	"github.com/Velocidex/ordereddict"
	"www.velocidex.com/golang/velociraptor/acls"
	"www.velocidex.com/golang/velociraptor/services"
	vql_subsystem "www.velocidex.com/golang/velociraptor/vql"
	"www.velocidex.com/golang/vfilter"
)

var FavoriteTypes = []string{"CLIENT", "SERVER", "CLIENT_EVENT", "SERVER_EVENT"}

type ListFavoritesPlugin struct{}

func (self ListFavoritesPlugin) Call(
	ctx context.Context,
	scope vfilter.Scope,
	args *ordereddict.Dict) <-chan vfilter.Row {
	output_chan := make(chan vfilter.Row)

	go func() {
		defer close(output_chan)
		defer vql_subsystem.RegisterMonitor(ctx, "favorites_list", args)()

		err := services.RequireFrontend()
		if err != nil {
			scope.Log("favorites_list: %v", err)
			return
		}

		// Favorites are user preferences - so everyone has permission
		// to add their own favorites.
		config_obj, ok := vql_subsystem.GetServerConfig(scope)
		if !ok {
			scope.Log("favorites_save: Command can only run on the server")
			return
		}

		err = vql_subsystem.CheckAccess(scope, acls.READ_RESULTS)
		if err != nil {
			scope.Log("flows: %s", err)
			return
		}

		principal := vql_subsystem.GetPrincipal(scope)
		if principal == "" {
			scope.Log("favorites_list: Username not specified")
			return
		}

		users_manager := services.GetUserManager()
		for _, t := range FavoriteTypes {
			favs, err := users_manager.GetFavorites(ctx, config_obj, principal, t)
			if err != nil {
				scope.Log("favorites_list: %v", err)
				continue
			}

			for _, fav := range favs.Items {
				select {
				case <-ctx.Done():
					return
				case output_chan <- fav:
				}

			}
		}
	}()

	return output_chan
}

func (self ListFavoritesPlugin) Info(
	scope vfilter.Scope, type_map *vfilter.TypeMap) *vfilter.PluginInfo {
	return &vfilter.PluginInfo{
		Name: "favorites_list",
		Doc:  "List all user's favorites.",
	}
}

func init() {
	vql_subsystem.RegisterPlugin(&ListFavoritesPlugin{})
}
