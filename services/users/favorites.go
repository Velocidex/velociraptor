package users

import (
	api_proto "www.velocidex.com/golang/velociraptor/api/proto"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	datastore "www.velocidex.com/golang/velociraptor/datastore"
	"www.velocidex.com/golang/velociraptor/paths"
)

func (self UserManager) GetFavorites(
	config_obj *config_proto.Config,
	principal, fav_type string) (*api_proto.Favorites, error) {
	result := &api_proto.Favorites{}
	path_manager := paths.NewUserPathManager(principal)

	db, err := datastore.GetDB(config_obj)
	if err != nil {
		return nil, err
	}

	components := path_manager.FavoriteDir(fav_type)
	children, err := db.ListChildren(config_obj, components)
	if err != nil {
		return nil, err
	}

	for _, child := range children {
		if child.IsDir() {
			continue
		}

		fav := &api_proto.Favorite{}
		err = db.GetSubject(config_obj,
			path_manager.Favorites(child.Base(), fav_type), fav)
		if err == nil {
			result.Items = append(result.Items, fav)
		}
	}

	return result, nil
}
