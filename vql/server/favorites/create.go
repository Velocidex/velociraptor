package favorites

import (
	"context"

	"github.com/Velocidex/ordereddict"
	api_proto "www.velocidex.com/golang/velociraptor/api/proto"
	"www.velocidex.com/golang/velociraptor/datastore"
	"www.velocidex.com/golang/velociraptor/paths"
	"www.velocidex.com/golang/velociraptor/services"
	vql_subsystem "www.velocidex.com/golang/velociraptor/vql"
	"www.velocidex.com/golang/velociraptor/vql/tools/collector"
	vql_utils "www.velocidex.com/golang/velociraptor/vql/utils"
	"www.velocidex.com/golang/vfilter"
	"www.velocidex.com/golang/vfilter/arg_parser"
)

type AddFavoriteArgs struct {
	Name        string      `vfilter:"required,field=name,doc=A name for this collection template."`
	Description string      `vfilter:"optional,field=description,doc=A description for the template."`
	Specs       vfilter.Any `vfilter:"required,field=specs,doc=The collection request spec that will be saved. We use this to create the new collection."`
	Type        string      `vfilter:"required,field=type,doc=The type of favorite."`
}

type AddFavorite struct{}

func (self *AddFavorite) Call(ctx context.Context,
	scope vfilter.Scope,
	args *ordereddict.Dict) vfilter.Any {

	arg := &AddFavoriteArgs{}
	err := arg_parser.ExtractArgsWithContext(ctx, scope, args, arg)
	if err != nil {
		scope.Log("favorites_save: %s", err.Error())
		return vfilter.Null{}
	}

	err = services.RequireFrontend()
	if err != nil {
		scope.Log("favorites_save: %v", err)
		return vfilter.Null{}
	}

	// Favorites are user preferences - so everyone has permission
	// to add their own favorites.
	config_obj, ok := vql_subsystem.GetServerConfig(scope)
	if !ok {
		scope.Log("favorites_save: Command can only run on the server")
		return vfilter.Null{}
	}

	repository, err := vql_utils.GetRepository(scope)
	if err != nil {
		scope.Log("favorites_save: Command can only run on the server")
		return vfilter.Null{}
	}

	// Validate the artifact collector spec
	specs, _, err := collector.AddSpecProtobuf(
		ctx, config_obj, repository, scope, arg.Specs, nil)
	if err != nil {
		scope.Log("favorites_save: %s", err)
		return vfilter.Null{}
	}

	db, err := datastore.GetDB(config_obj)
	if err != nil {
		scope.Log("favorites_save: %s", err)
		return vfilter.Null{}
	}

	principal := vql_subsystem.GetPrincipal(scope)
	if principal == "" {
		scope.Log("Username not specified")
		return vfilter.Null{}
	}

	path_manager := paths.NewUserPathManager(principal)
	fav := &api_proto.Favorite{
		Name:        arg.Name,
		Description: arg.Description,
		Spec:        specs,
		Type:        arg.Type,
	}
	err = db.SetSubject(config_obj,
		path_manager.Favorites(arg.Name, arg.Type), fav)
	if err != nil {
		scope.Log("favorites_save: %s", err)
		return vfilter.Null{}
	}
	return fav
}

func (self AddFavorite) Info(
	scope vfilter.Scope, type_map *vfilter.TypeMap) *vfilter.FunctionInfo {
	return &vfilter.FunctionInfo{
		Name:    "favorites_save",
		Doc:     "Save a collection into the favorites.",
		ArgType: type_map.AddType(scope, &AddFavoriteArgs{}),
	}
}

func init() {
	vql_subsystem.RegisterFunction(&AddFavorite{})
}
