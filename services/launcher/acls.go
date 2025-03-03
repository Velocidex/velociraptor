package launcher

import (
	"fmt"

	"www.velocidex.com/golang/velociraptor/acls"
	artifacts_proto "www.velocidex.com/golang/velociraptor/artifacts/proto"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	flows_proto "www.velocidex.com/golang/velociraptor/flows/proto"
	vql_subsystem "www.velocidex.com/golang/velociraptor/vql"
)

func CheckAccess(
	config_obj *config_proto.Config,
	artifact *artifacts_proto.Artifact,
	collector_request *flows_proto.ArtifactCollectorArgs,
	acl_manager vql_subsystem.ACLManager) error {

	// Check if the user has COLLECT_BASIC permission, as we will need
	// to do additional checks.
	perm, err := acl_manager.CheckAccess(acls.COLLECT_BASIC)
	if err == nil && perm {
		// COLLECT_BASIC permission is sufficient to override
		// artifact's required permission. The usecase is that low
		// privilege users are given only COLLECT_BASIC and then only
		// certain artifacts are marked as BASIC. If we required the
		// users to also contain higher permissions it would defeat
		// the purpose of the COLLECT_BASIC mechanism because the user
		// will also need to be given extra permissions.
		err := checkBasicPermission(
			config_obj, artifact, collector_request, acl_manager)
		if err == nil {
			return nil
		}
		// Fallthrough in case user also has COLLECT_CLIENT
	}

	permissions := acls.COLLECT_CLIENT
	if collector_request.ClientId == "server" {
		permissions = acls.COLLECT_SERVER
	}

	// If the user has COLLECT_CLIENT or COLLECT_SERVER they can
	// collect anything but if they dont we allow the user to have the
	// lesser COLLECT_BASIC permission which requires a check on the
	// artifact metadata.
	perm, err = acl_manager.CheckAccess(permissions)
	if !perm || err != nil {

		// The user can not directly launch the artifact but maybe the
		// artifact is marked as Basic and users have the
		// COLLECT_BASIC permission.
		if artifact.Metadata != nil &&
			artifact.Metadata.Basic {
			perm, err = acl_manager.CheckAccess(acls.COLLECT_BASIC)
		}

		if !perm || err != nil {
			principal := ""
			p_acl, ok := acl_manager.(vql_subsystem.PrincipalACLManager)
			if ok {
				principal = p_acl.GetPrincipal()
			}

			return fmt.Errorf(
				"%w: User %v is not allowed to launch flows %v.",
				acls.PermissionDenied, principal, permissions)
		}
	}

	if artifact.RequiredPermissions != nil {
		err := checkRequiredPermissions(config_obj,
			artifact, acl_manager)
		if err != nil {
			return err
		}
	}

	// User is allowed
	return nil
}

func checkRequiredPermissions(
	config_obj *config_proto.Config,
	artifact *artifacts_proto.Artifact,
	acl_manager vql_subsystem.ACLManager) error {
	// Principal must have ALL permissions to succeed.
	for _, perm := range artifact.RequiredPermissions {
		permission := acls.GetPermission(perm)
		perm, err := acl_manager.CheckAccess(permission)
		if !perm {
			if err != nil {
				return fmt.Errorf(
					"%w: While collecting artifact (%s) permission denied %v",
					err, artifact.Name, permission)
			}
			return fmt.Errorf(
				"%w: While collecting artifact (%s) permission denied %v",
				acls.PermissionDenied, artifact.Name, permission)
		}
	}

	return nil
}

func checkBasicPermission(
	config_obj *config_proto.Config,
	artifact *artifacts_proto.Artifact,
	collector_request *flows_proto.ArtifactCollectorArgs,
	acl_manager vql_subsystem.ACLManager) error {

	if artifact.Metadata != nil && artifact.Metadata.Basic {
		return nil
	}

	return acls.PermissionDenied
}
