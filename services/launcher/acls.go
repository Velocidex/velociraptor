package launcher

import (
	"fmt"

	errors "github.com/pkg/errors"
	"www.velocidex.com/golang/velociraptor/acls"
	artifacts_proto "www.velocidex.com/golang/velociraptor/artifacts/proto"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	vql_subsystem "www.velocidex.com/golang/velociraptor/vql"
)

func CheckAccess(
	config_obj *config_proto.Config,
	artifact *artifacts_proto.Artifact,
	acl_manager vql_subsystem.ACLManager) error {

	if artifact.RequiredPermissions == nil {
		return nil
	}

	// Principal must have ALL permissions to succeed.
	for _, perm := range artifact.RequiredPermissions {
		permission := acls.GetPermission(perm)
		perm, err := acl_manager.CheckAccess(permission)
		if !perm || err != nil {
			return errors.New(fmt.Sprintf(
				"While collecting artifact (%s) permission denied %v",
				artifact.Name, permission))
		}
	}

	return nil
}
