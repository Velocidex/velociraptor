package api

import (
	context "golang.org/x/net/context"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"www.velocidex.com/golang/velociraptor/acls"
	artifacts_proto "www.velocidex.com/golang/velociraptor/artifacts/proto"
	"www.velocidex.com/golang/velociraptor/services"
	users "www.velocidex.com/golang/velociraptor/users"
)

func (self *ApiServer) GetToolInfo(ctx context.Context,
	in *artifacts_proto.Tool) (*artifacts_proto.Tool, error) {
	user_name := GetGRPCUserInfo(self.config, ctx).Name
	user_record, err := users.GetUser(self.config, user_name)
	if err != nil {
		return nil, err
	}

	permissions := acls.READ_RESULTS
	perm, err := acls.CheckAccess(self.config, user_record.Name, permissions)
	if !perm || err != nil {
		return nil, status.Error(codes.PermissionDenied,
			"User is not allowed to view tools.")
	}

	return services.GetInventory().ProbeToolInfo(in.Name)
}

func (self *ApiServer) SetToolInfo(ctx context.Context,
	in *artifacts_proto.Tool) (*artifacts_proto.Tool, error) {
	user_name := GetGRPCUserInfo(self.config, ctx).Name
	user_record, err := users.GetUser(self.config, user_name)
	if err != nil {
		return nil, err
	}

	// Minimum permission required. If the user can write
	// artifacts they can already autoload tools by uploading an
	// artifact definition.
	permissions := acls.ARTIFACT_WRITER
	perm, err := acls.CheckAccess(self.config, user_record.Name, permissions)
	if !perm || err != nil {
		return nil, status.Error(codes.PermissionDenied,
			"User is not allowed to update tool definitions.")
	}

	materialize := in.Materialize
	in.Materialize = false
	err = services.GetInventory().AddTool(self.config, in)
	if err != nil {
		return nil, err
	}

	// If materialized we re-fetch the tool and send back the full
	// record.
	if materialize {
		return services.GetInventory().GetToolInfo(ctx, self.config,
			in.Name)
	}

	return in, nil
}
