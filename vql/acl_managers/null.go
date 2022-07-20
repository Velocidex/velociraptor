package acl_managers

import "www.velocidex.com/golang/velociraptor/acls"

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
