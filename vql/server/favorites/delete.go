package favorites

import (
	"context"

	"github.com/Velocidex/ordereddict"
	"www.velocidex.com/golang/velociraptor/datastore"
	"www.velocidex.com/golang/velociraptor/paths"
	"www.velocidex.com/golang/velociraptor/services"
	vql_subsystem "www.velocidex.com/golang/velociraptor/vql"
	"www.velocidex.com/golang/vfilter"
	"www.velocidex.com/golang/vfilter/arg_parser"
)

type RmFavoriteArgs struct {
	Name string `vfilter:"required,field=name,doc=A name for this collection template."`
	Type string `vfilter:"required,field=type,doc=The type of favorite."`
}

type RmFavorite struct{}

func (self *RmFavorite) Call(ctx context.Context,
	scope vfilter.Scope,
	args *ordereddict.Dict) vfilter.Any {

	arg := &RmFavoriteArgs{}
	err := arg_parser.ExtractArgsWithContext(ctx, scope, args, arg)
	if err != nil {
		scope.Log("favorites_delete: %v", err)
		return vfilter.Null{}
	}

	err = services.RequireFrontend()
	if err != nil {
		scope.Log("favorites_delete: %v", err)
		return vfilter.Null{}
	}

	// Favorites are user preferences - so everyone has permission
	// to add their own favorites.
	config_obj, ok := vql_subsystem.GetServerConfig(scope)
	if !ok {
		scope.Log("favorites_delete: Command can only run on the server")
		return vfilter.Null{}
	}

	db, err := datastore.GetDB(config_obj)
	if err != nil {
		scope.Log("favorites_delete: %s", err)
		return vfilter.Null{}
	}

	principal := vql_subsystem.GetPrincipal(scope)
	if principal == "" {
		scope.Log("favorites_delete: Username not specified")
		return vfilter.Null{}
	}

	path_manager := paths.NewUserPathManager(principal)
	err = db.DeleteSubject(config_obj,
		path_manager.Favorites(arg.Name, arg.Type))
	if err != nil {
		scope.Log("favorites_delete: %s", err)
		return vfilter.Null{}
	}
	return vfilter.Null{}
}

func (self RmFavorite) Info(
	scope vfilter.Scope, type_map *vfilter.TypeMap) *vfilter.FunctionInfo {
	return &vfilter.FunctionInfo{
		Name:    "favorites_delete",
		Doc:     "Delete a favorite.",
		ArgType: type_map.AddType(scope, &RmFavoriteArgs{}),
	}
}

func init() {
	vql_subsystem.RegisterFunction(&RmFavorite{})
}
