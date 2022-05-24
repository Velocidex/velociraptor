package api

import (
	context "golang.org/x/net/context"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/emptypb"
	"www.velocidex.com/golang/velociraptor/acls"
	api_proto "www.velocidex.com/golang/velociraptor/api/proto"
	"www.velocidex.com/golang/velociraptor/services"
)

func (self *ApiServer) GetUsers(
	ctx context.Context,
	in *emptypb.Empty) (*api_proto.Users, error) {

	users_manager := services.GetUserManager()
	user_record, err := users_manager.GetUserFromContext(self.config, ctx)
	if err != nil {
		return nil, err
	}

	permissions := acls.READ_RESULTS
	perm, err := acls.CheckAccess(self.config, user_record.Name, permissions)
	if !perm || err != nil {
		return nil, status.Error(codes.PermissionDenied,
			"User is not allowed to enumerate users.")
	}

	result := &api_proto.Users{}

	users, err := users_manager.ListUsers(self.config)
	if err != nil {
		return nil, err
	}

	result.Users = users

	return result, nil
}

func (self *ApiServer) GetUserFavorites(
	ctx context.Context,
	in *api_proto.Favorite) (*api_proto.Favorites, error) {

	// No special permission requires to view a user's own favorites.
	users_manager := services.GetUserManager()
	user_record, err := users_manager.GetUserFromContext(self.config, ctx)
	if err != nil {
		return nil, err
	}
	user_name := user_record.Name
	return users_manager.GetFavorites(self.config, user_name, in.Type)
}
