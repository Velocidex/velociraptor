package api

import (
	context "golang.org/x/net/context"
	"www.velocidex.com/golang/velociraptor/acls"
	api_proto "www.velocidex.com/golang/velociraptor/api/proto"
	"www.velocidex.com/golang/velociraptor/services"
)

func (self *ApiServer) ReformatVQL(
	ctx context.Context,
	in *api_proto.ReformatVQLMessage) (*api_proto.ReformatVQLMessage, error) {

	// Empty creators are called internally.
	users := services.GetUserManager()
	user_record, org_config_obj, err := users.GetUserFromContext(ctx)
	if err != nil {
		return nil, err
	}

	permissions := acls.READ_RESULTS
	perm, err := services.CheckAccess(org_config_obj, user_record.Name, permissions)
	if !perm || err != nil {
		return nil, PermissionDenied(err,
			"User is not allowed to read notebooks.")
	}

	notebook_manager, err := services.GetNotebookManager(org_config_obj)
	if err != nil {
		return nil, Status(self.verbose, err)
	}

	reformated_vql, err := notebook_manager.ReformatVQL(ctx, in.Vql)
	return &api_proto.ReformatVQLMessage{
		Vql: reformated_vql,
	}, Status(self.verbose, err)
}
