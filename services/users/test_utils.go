package users

import (
	"context"
	"crypto/x509"

	api_proto "www.velocidex.com/golang/velociraptor/api/proto"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	"www.velocidex.com/golang/velociraptor/services"
)

type TestUserManager struct {
	*UserManager
	username string
}

func (self TestUserManager) GetUserFromContext(ctx context.Context) (
	*api_proto.VelociraptorUser, *config_proto.Config, error) {
	return &api_proto.VelociraptorUser{
		Name: self.username,
	}, self.config_obj, nil
}

func RegisterTestUserManager(config_obj *config_proto.Config, username string) {
	services.RegisterUserManager(&TestUserManager{
		username: username,
		UserManager: &UserManager{
			ca_pool:    x509.NewCertPool(),
			config_obj: config_obj,
		}})
}
