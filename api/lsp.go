package api

import (
	"context"

	"www.velocidex.com/golang/velociraptor/acls"
	api_proto "www.velocidex.com/golang/velociraptor/api/proto"
	"www.velocidex.com/golang/velociraptor/services"
)

func (self *ApiServer) LSP(
	ctx context.Context,
	in *api_proto.LSPRequest) (*api_proto.LSPResponse, error) {

	defer Instrument("LSP")()

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
			"User is not allowed to issue LSP queries.")
	}

	lsp_service, err := services.GetLSPServer(org_config_obj)
	if err != nil {
		return nil, Status(self.verbose, err)
	}

	return lsp_service.LSP(ctx, in)
}
