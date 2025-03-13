package api

import (
	"context"

	"www.velocidex.com/golang/velociraptor/acls"
	artifacts_proto "www.velocidex.com/golang/velociraptor/artifacts/proto"
	"www.velocidex.com/golang/velociraptor/services"
)

func (self *ApiServer) GetToolInfo(ctx context.Context,
	in *artifacts_proto.Tool) (*artifacts_proto.Tool, error) {

	defer Instrument("GetToolInfo")()

	users := services.GetUserManager()
	user_record, org_config_obj, err := users.GetUserFromContext(ctx)
	if err != nil {
		return nil, Status(self.verbose, err)
	}

	permissions := acls.READ_RESULTS
	perm, err := services.CheckAccess(org_config_obj, user_record.Name, permissions)
	if !perm || err != nil {
		return nil, PermissionDenied(err,
			"User is not allowed to view tools.")
	}

	inventory, err := services.GetInventory(org_config_obj)
	if err != nil {
		return nil, Status(self.verbose, err)
	}
	if in.Materialize {
		return inventory.GetToolInfo(ctx, org_config_obj, in.Name, in.Version)
	}

	tool, err := inventory.ProbeToolInfo(ctx, org_config_obj, in.Name, in.Version)
	return tool, Status(self.verbose, err)
}

func (self *ApiServer) SetToolInfo(ctx context.Context,
	in *artifacts_proto.Tool) (*artifacts_proto.Tool, error) {

	defer Instrument("SetToolInfo")()

	users := services.GetUserManager()
	user_record, org_config_obj, err := users.GetUserFromContext(ctx)
	if err != nil {
		return nil, Status(self.verbose, err)
	}

	// Minimum permission required. If the user can write
	// artifacts they can already autoload tools by uploading an
	// artifact definition.
	permissions := acls.ARTIFACT_WRITER
	perm, err := services.CheckAccess(org_config_obj, user_record.Name, permissions)
	if !perm || err != nil {
		return nil, PermissionDenied(err,
			"User is not allowed to update tool definitions.")
	}

	materialize := in.Materialize
	in.Materialize = false

	inventory, err := services.GetInventory(org_config_obj)
	if err != nil {
		return nil, Status(self.verbose, err)
	}

	// Clear internally managed fields the user should not be allowed
	// to set.
	in.Versions = nil
	in.ServeUrl = ""
	in.InvalidHash = ""

	err = inventory.AddTool(ctx, org_config_obj, in,
		services.ToolOptions{
			AdminOverride: true,
		})
	if err != nil {
		return nil, Status(self.verbose, err)
	}

	// If materialized we re-fetch the tool and send back the full
	// record.
	if materialize {
		return inventory.GetToolInfo(ctx, org_config_obj, in.Name, in.Version)
	}

	return in, nil
}
