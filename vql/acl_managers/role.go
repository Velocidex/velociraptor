package acl_managers

import (
	"www.velocidex.com/golang/velociraptor/acls"
	acl_proto "www.velocidex.com/golang/velociraptor/acls/proto"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	"www.velocidex.com/golang/velociraptor/services"
	"www.velocidex.com/golang/velociraptor/utils"
	vql_subsystem "www.velocidex.com/golang/velociraptor/vql"
)

// Satisfy the interface vql_subsystem.ACLManager
type RoleACLManager struct {
	Token      *acl_proto.ApiClientACL
	config_obj *config_proto.Config

	is_admin bool
}

func (self *RoleACLManager) CheckAccess(
	permissions ...acls.ACL_PERMISSION) (bool, error) {

	for _, permission := range permissions {
		ok, err := services.CheckAccessWithToken(self.Token, permission)
		if !ok || err != nil {
			return ok, err
		}
	}

	return true, nil
}

func (self *RoleACLManager) GetPrincipal() string {
	if self.is_admin {
		return utils.GetSuperuserName(self.config_obj)
	}
	return ""
}

// We have the same roles in all orgs
func (self *RoleACLManager) CheckAccessInOrg(
	org_id string, permission ...acls.ACL_PERMISSION) (bool, error) {
	return self.CheckAccess(permission...)
}

// NOOP because we always use the same token for all comparisons.
func (self *RoleACLManager) SwitchDefaultOrg(config_obj *config_proto.Config) {
}

func (self *RoleACLManager) CheckAccessWithArgs(
	permission acls.ACL_PERMISSION, args ...string) (bool, error) {

	return services.CheckAccessWithToken(self.Token, permission, args...)
}

// NewRoleACLManager creates an ACL manager with only the assigned
// roles. This is useful for creating limited VQL permissions
// internally.
func NewRoleACLManager(
	config_obj *config_proto.Config,
	roles ...string) vql_subsystem.ACLManager {
	policy := &acl_proto.ApiClientACL{}

	// If we fail just return an empty policy
	for _, role := range roles {
		_ = acls.GetRolePermissions(nil, []string{role}, policy)
	}
	policy.Roles = roles

	return &RoleACLManager{
		Token:      policy,
		config_obj: config_obj,
		is_admin:   utils.InString(roles, "administrator"),
	}
}
