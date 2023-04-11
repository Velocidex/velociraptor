package launcher

import (
	"fmt"

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
		if !perm {
			if err != nil {
				return fmt.Errorf(
					"While collecting artifact (%s) permission denied %v: %v",
					artifact.Name, permission, err)
			}
			return fmt.Errorf(
				"While collecting artifact (%s) permission denied %v",
				artifact.Name, permission)
		}
	}

	return nil
}
