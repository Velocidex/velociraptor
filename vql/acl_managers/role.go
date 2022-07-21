package acl_managers

import (
	"www.velocidex.com/golang/velociraptor/acls"
	acl_proto "www.velocidex.com/golang/velociraptor/acls/proto"
	vql_subsystem "www.velocidex.com/golang/velociraptor/vql"
)

type RoleACLManager struct {
	Token *acl_proto.ApiClientACL
}

func (self *RoleACLManager) CheckAccess(
	permissions ...acls.ACL_PERMISSION) (bool, error) {
	for _, permission := range permissions {
		ok, err := acls.CheckAccessWithToken(self.Token, permission)
		if !ok || err != nil {
			return ok, err
		}
	}

	return true, nil
}

func (self *RoleACLManager) CheckAccessWithArgs(
	permission acls.ACL_PERMISSION, args ...string) (bool, error) {

	return acls.CheckAccessWithToken(self.Token, permission, args...)
}

// NewRoleACLManager creates an ACL manager with only the assigned
// roles. This is useful for creating limited VQL permissions
// internally.
func NewRoleACLManager(roles ...string) vql_subsystem.ACLManager {
	policy := &acl_proto.ApiClientACL{}

	// If we fail just return an empty policy
	for _, role := range roles {
		_ = acls.GetRolePermissions(nil, []string{role}, policy)
	}
	return &RoleACLManager{Token: policy}
}
