package vql

import (
	"fmt"

	"www.velocidex.com/golang/velociraptor/acls"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	"www.velocidex.com/golang/vfilter"
)

const (
	ACL_MANAGER_VAR = "$acl"
)

type ACLManager interface {
	CheckAccess(permission ...acls.ACL_PERMISSION) (bool, error)

	// Extended check with extra args (Used for PUBLISH)
	CheckAccessWithArgs(
		permission acls.ACL_PERMISSION, args ...string) (bool, error)
}

type OrgACLManager interface {
	CheckAccessInOrg(org_id string, permission ...acls.ACL_PERMISSION) (bool, error)

	SwitchDefaultOrg(config_obj *config_proto.Config)
}

type PrincipalACLManager interface {
	GetPrincipal() string
}

// Check access through the ACL manager in the scope.  NOTE: This
// assumes it is not possible for a user to mask the ACL manager in
// the scope! There is currently no way to create an acl manager type
// from within VQL so this is a safe assumption - if a user was to
// override the ACL_MANAGER_VAR with something else this will lock
// down the entire VQL ACL system and deny all permissions.
func CheckAccess(scope vfilter.Scope, permissions ...acls.ACL_PERMISSION) error {
	manager_any, pres := scope.Resolve(ACL_MANAGER_VAR)
	if !pres {
		return fmt.Errorf("%w: Permission denied: %v",
			acls.PermissionDenied, permissions)
	}

	manager, ok := manager_any.(ACLManager)
	if !ok {
		return fmt.Errorf("%w: Permission denied: %v",
			acls.PermissionDenied, permissions)
	}

	perm, err := manager.CheckAccess(permissions...)
	if !perm {
		if err == nil {
			return fmt.Errorf("%w: Permission denied: %v",
				acls.PermissionDenied, permissions)
		}
		return fmt.Errorf("%w: %v", err, permissions)
	}

	return nil
}

// A variant of CheckAccess() that can check access in a different org.
func CheckAccessInOrg(scope vfilter.Scope, org_id string, permissions ...acls.ACL_PERMISSION) error {
	manager_any, pres := scope.Resolve(ACL_MANAGER_VAR)
	if !pres {
		return fmt.Errorf("Permission denied: %v", permissions)
	}

	manager, ok := manager_any.(OrgACLManager)
	if !ok {
		return fmt.Errorf("Permission denied: %v", permissions)
	}

	if org_id == "root" {
		org_id = ""
	}

	perm, err := manager.CheckAccessInOrg(org_id, permissions...)
	if !perm {
		if err != nil {
			return fmt.Errorf("%v: %v", err, permissions)
		}
		return fmt.Errorf("Permission denied: %v", permissions)
	}

	return nil
}

func CheckAccessWithArgs(scope vfilter.Scope, permissions acls.ACL_PERMISSION,
	args ...string) error {
	manager_any, pres := scope.Resolve(ACL_MANAGER_VAR)
	if !pres {
		return fmt.Errorf("Permission denied: %v", permissions)
	}

	manager, ok := manager_any.(ACLManager)
	if !ok {
		return fmt.Errorf("Permission denied: %v", permissions)
	}

	perm, err := manager.CheckAccessWithArgs(permissions, args...)
	if !perm {
		if err != nil {
			return fmt.Errorf("%v: %v", err, permissions)
		}
		return fmt.Errorf("Permission denied: %v", permissions)
	}

	return nil
}

// Get the principal that is running the query if possible.
func GetPrincipal(scope vfilter.Scope) string {
	manager_any, pres := scope.Resolve(ACL_MANAGER_VAR)
	if !pres {
		return ""
	}

	manager, ok := manager_any.(PrincipalACLManager)
	if !ok {
		return ""
	}

	return manager.GetPrincipal()
}
