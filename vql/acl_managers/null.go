package acl_managers

import (
	"www.velocidex.com/golang/velociraptor/acls"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	"www.velocidex.com/golang/velociraptor/constants"
)

// Satisfy the interface vql_subsystem.ACLManager

// NullACLManager is an acl manager which allows everything. This is
// currently used on the client and on the command line where there is
// no clear principal or ACL controls.
type NullACLManager struct{}

func (self NullACLManager) CheckAccess(
	permission ...acls.ACL_PERMISSION) (bool, error) {
	return true, nil
}

func (self NullACLManager) CheckAccessWithArgs(
	permission acls.ACL_PERMISSION, args ...string) (bool, error) {
	return true, nil
}

// NOOP
func (self NullACLManager) SwitchDefaultOrg(config_obj *config_proto.Config) {}

func (self NullACLManager) CheckAccessInOrg(
	org_id string, permission ...acls.ACL_PERMISSION) (bool, error) {
	return true, nil
}

func (self NullACLManager) GetPrincipal() string {
	return constants.PinnedServerName
}
