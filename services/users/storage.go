package users

import (
	"context"
	"errors"

	api_proto "www.velocidex.com/golang/velociraptor/api/proto"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
)

// Responsible for storing the User records
type IUserStorageManager interface {
	GetUserWithHashes(ctx context.Context, username string) (
		*api_proto.VelociraptorUser, error)

	SetUser(ctx context.Context, user_record *api_proto.VelociraptorUser) error

	ListAllUsers(ctx context.Context) ([]*api_proto.VelociraptorUser, error)

	GetUserOptions(ctx context.Context, username string) (
		*api_proto.SetGUIOptionsRequest, error)

	SetUserOptions(ctx context.Context,
		username string, options *api_proto.SetGUIOptionsRequest) error

	// Favourites are stored per org.
	GetFavorites(
		ctx context.Context,
		org_config_obj *config_proto.Config,
		principal, fav_type string) (*api_proto.Favorites, error)

	DeleteUser(ctx context.Context, username string) error
}

// The NullStorage Manager is used for tools and clients. In this
// configuration there are no users and none of the user based VQL
// plugins will work.
type NullStorageManager struct{}

func (self *NullStorageManager) GetUserWithHashes(ctx context.Context, username string) (
	*api_proto.VelociraptorUser, error) {
	return nil, errors.New("Not Found")
}

func (self *NullStorageManager) SetUser(ctx context.Context,
	user_record *api_proto.VelociraptorUser) error {
	return errors.New("Not Implemented")
}

func (self *NullStorageManager) ListAllUsers(
	ctx context.Context) ([]*api_proto.VelociraptorUser, error) {
	return nil, errors.New("Not Implemented")
}

func (self *NullStorageManager) GetUserOptions(ctx context.Context, username string) (
	*api_proto.SetGUIOptionsRequest, error) {
	return nil, errors.New("Not Implemented")
}

func (self *NullStorageManager) SetUserOptions(ctx context.Context,
	username string, options *api_proto.SetGUIOptionsRequest) error {
	return errors.New("Not Implemented")
}

func (self *NullStorageManager) DeleteUser(ctx context.Context, username string) error {
	return errors.New("Not Implemented")
}

func (self *NullStorageManager) GetFavorites(
	ctx context.Context, org_config_obj *config_proto.Config,
	principal, fav_type string) (*api_proto.Favorites, error) {
	return nil, errors.New("Not Implemented")
}
