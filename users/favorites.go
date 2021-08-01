package users

import (
	api_proto "www.velocidex.com/golang/velociraptor/api/proto"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	datastore "www.velocidex.com/golang/velociraptor/datastore"
	"www.velocidex.com/golang/velociraptor/paths"
)

func GetFavorites(
	config_obj *config_proto.Config,
	principal, fav_type string) (*api_proto.Favorites, error) {
	result := &api_proto.Favorites{}
	path_manager := paths.NewUserPathManager(principal)

	db, err := datastore.GetDB(config_obj)
	if err != nil {
		return nil, err
	}

	components := path_manager.FavoriteDir(fav_type)
	children, err := db.ListChildrenJSON(config_obj, components)
	if err != nil {
		return nil, err
	}

	for _, child := range children {
		fav := &api_proto.Favorite{}
		err = db.GetSubjectJSON(config_obj,
			path_manager.Favorites(child.Name, fav_type), fav)
		if err == nil {
			result.Items = append(result.Items, fav)
		}
	}

	return result, nil
}
