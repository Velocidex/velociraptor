package api

import (
	errors "github.com/pkg/errors"
	"github.com/sirupsen/logrus"
	context "golang.org/x/net/context"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/emptypb"
	"www.velocidex.com/golang/velociraptor/acls"
	api_proto "www.velocidex.com/golang/velociraptor/api/proto"
	"www.velocidex.com/golang/velociraptor/logging"
	"www.velocidex.com/golang/velociraptor/services"
	"www.velocidex.com/golang/velociraptor/services/users"
)

// This is only used to set the user's own password which is always
// allowed for any user.
func (self *ApiServer) SetPassword(
	ctx context.Context,
	in *api_proto.SetPasswordRequest) (*emptypb.Empty, error) {

	// Enforce a minimum length password
	if len(in.Password) < 4 {
		return nil, errors.New("Password is not set or too short")
	}

	users_manager := services.GetUserManager()
	user_record, _, err := users_manager.GetUserFromContext(ctx)
	if err != nil {
		return nil, err
	}

	// Set the password on the record.
	users.SetPassword(user_record, in.Password)

	org_manager, err := services.GetOrgManager()
	if err != nil {
		return nil, err
	}

	org_config_obj, err := org_manager.GetOrgConfig(services.ROOT_ORG_ID)
	if err != nil {
		return nil, err
	}

	logger := logging.GetLogger(org_config_obj, &logging.Audit)
	logger.WithFields(logrus.Fields{
		"Username":  user_record.Name,
		"Principal": user_record.Name,
	}).Info("passwd: Updating password for user via API")

	// Store the record
	return &emptypb.Empty{}, users_manager.SetUser(user_record)
}

func (self *ApiServer) GetUsers(
	ctx context.Context,
	in *emptypb.Empty) (*api_proto.Users, error) {

	users_manager := services.GetUserManager()
	user_record, org_config_obj, err := users_manager.GetUserFromContext(ctx)
	if err != nil {
		return nil, err
	}

	permissions := acls.READ_RESULTS
	perm, err := acls.CheckAccess(org_config_obj, user_record.Name, permissions)
	if !perm || err != nil {
		return nil, status.Error(codes.PermissionDenied,
			"User is not allowed to enumerate users.")
	}

	result := &api_proto.Users{}

	users, err := users_manager.ListUsers()
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
	user_record, org_config_obj, err := users_manager.GetUserFromContext(ctx)
	if err != nil {
		return nil, err
	}
	user_name := user_record.Name
	return users_manager.GetFavorites(org_config_obj, user_name, in.Type)
}
