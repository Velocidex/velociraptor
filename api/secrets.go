package api

import (
	"context"

	"github.com/Velocidex/ordereddict"
	"google.golang.org/protobuf/types/known/emptypb"
	"www.velocidex.com/golang/velociraptor/acls"
	api_proto "www.velocidex.com/golang/velociraptor/api/proto"
	"www.velocidex.com/golang/velociraptor/services"
	vql_subsystem "www.velocidex.com/golang/velociraptor/vql"
)

func (self *ApiServer) GetSecretDefinitions(
	ctx context.Context,
	in *emptypb.Empty) (*api_proto.SecretDefinitionList, error) {

	users := services.GetUserManager()
	user_record, org_config_obj, err := users.GetUserFromContext(ctx)
	if err != nil {
		return nil, Status(self.verbose, err)
	}
	principal := user_record.Name

	permissions := acls.READ_RESULTS
	perm, err := services.CheckAccess(org_config_obj, principal, permissions)
	if !perm || err != nil {
		return nil, PermissionDenied(err,
			"User is not allowed to view secrets.")
	}

	secrets, err := services.GetSecretsService(org_config_obj)
	if err != nil {
		return nil, Status(self.verbose, err)
	}

	return &api_proto.SecretDefinitionList{
		Items: secrets.GetSecretDefinitions(ctx),
	}, nil

}

func (self *ApiServer) DefineSecret(
	ctx context.Context,
	in *api_proto.SecretDefinition) (*emptypb.Empty, error) {

	users := services.GetUserManager()
	user_record, org_config_obj, err := users.GetUserFromContext(ctx)
	if err != nil {
		return nil, Status(self.verbose, err)
	}
	principal := user_record.Name

	permissions := acls.SERVER_ADMIN
	perm, err := services.CheckAccess(org_config_obj, principal, permissions)
	if !perm || err != nil {
		return nil, PermissionDenied(err,
			"User is not allowed to manage secrets.")
	}

	secrets, err := services.GetSecretsService(org_config_obj)
	if err != nil {
		return nil, Status(self.verbose, err)
	}

	err = secrets.DefineSecret(ctx, in)
	if err == nil {
		services.LogAudit(ctx,
			org_config_obj, principal, "User Defined Secret",
			ordereddict.NewDict().
				Set("principal", principal).
				Set("type", in.TypeName))
	}

	return &emptypb.Empty{}, Status(self.verbose, err)
}

func (self *ApiServer) DeleteSecretDefinition(
	ctx context.Context,
	in *api_proto.SecretDefinition) (*emptypb.Empty, error) {

	users := services.GetUserManager()
	user_record, org_config_obj, err := users.GetUserFromContext(ctx)
	if err != nil {
		return nil, Status(self.verbose, err)
	}
	principal := user_record.Name

	permissions := acls.SERVER_ADMIN
	perm, err := services.CheckAccess(org_config_obj, principal, permissions)
	if !perm || err != nil {
		return nil, PermissionDenied(err,
			"User is not allowed to manage secrets.")
	}

	secrets, err := services.GetSecretsService(org_config_obj)
	if err != nil {
		return nil, Status(self.verbose, err)
	}

	err = secrets.DeleteSecretDefinition(ctx, in)
	if err == nil {
		services.LogAudit(ctx,
			org_config_obj, principal, "User Deleted Secret Type",
			ordereddict.NewDict().
				Set("principal", principal).
				Set("type", in.TypeName))
	}

	return &emptypb.Empty{}, Status(self.verbose, err)
}

func (self *ApiServer) AddSecret(
	ctx context.Context,
	in *api_proto.Secret) (*emptypb.Empty, error) {

	users := services.GetUserManager()
	user_record, org_config_obj, err := users.GetUserFromContext(ctx)
	if err != nil {
		return nil, Status(self.verbose, err)
	}
	principal := user_record.Name

	permissions := acls.SERVER_ADMIN
	perm, err := services.CheckAccess(org_config_obj, principal, permissions)
	if !perm || err != nil {
		return nil, PermissionDenied(err,
			"User is not allowed to manage secrets.")
	}

	secrets, err := services.GetSecretsService(org_config_obj)
	if err != nil {
		return nil, Status(self.verbose, err)
	}

	secret_data := ordereddict.NewDict()
	for k, v := range in.Secret {
		secret_data.Set(k, v)
	}

	scope := vql_subsystem.MakeScope()
	err = secrets.AddSecret(ctx, scope, in.TypeName, in.Name, secret_data)
	return &emptypb.Empty{}, Status(self.verbose, err)
}

func (self *ApiServer) GetSecret(
	ctx context.Context,
	in *api_proto.Secret) (*api_proto.Secret, error) {

	users := services.GetUserManager()
	user_record, org_config_obj, err := users.GetUserFromContext(ctx)
	if err != nil {
		return nil, Status(self.verbose, err)
	}
	principal := user_record.Name

	permissions := acls.SERVER_ADMIN
	perm, err := services.CheckAccess(org_config_obj, principal, permissions)
	if !perm || err != nil {
		return nil, PermissionDenied(err,
			"User is not allowed to manage secrets.")
	}

	secrets, err := services.GetSecretsService(org_config_obj)
	if err != nil {
		return nil, Status(self.verbose, err)
	}

	secret, err := secrets.GetSecretMetadata(ctx, in.TypeName, in.Name)
	if err != nil {
		return nil, Status(self.verbose, err)
	}

	// Return a redacted version of the secret so the GUI can render
	// the secret metadata
	return secret.Secret, nil
}

func (self *ApiServer) ModifySecret(
	ctx context.Context,
	in *api_proto.ModifySecretRequest) (*emptypb.Empty, error) {

	users := services.GetUserManager()
	user_record, org_config_obj, err := users.GetUserFromContext(ctx)
	if err != nil {
		return nil, Status(self.verbose, err)
	}
	principal := user_record.Name

	permissions := acls.SERVER_ADMIN
	perm, err := services.CheckAccess(org_config_obj, principal, permissions)
	if !perm || err != nil {
		return nil, PermissionDenied(err,
			"User is not allowed to manage secrets.")
	}

	secrets, err := services.GetSecretsService(org_config_obj)
	if err != nil {
		return nil, Status(self.verbose, err)
	}

	err = secrets.ModifySecret(ctx, in)
	return &emptypb.Empty{}, Status(self.verbose, err)
}
