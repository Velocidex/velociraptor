package accessors

import (
	"www.velocidex.com/golang/velociraptor/acls"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	vql_subsystem "www.velocidex.com/golang/velociraptor/vql"
)

type RemappedACLManager struct{}

func (self RemappedACLManager) CheckAccess(permissions ...acls.ACL_PERMISSION) (
	bool, error) {
	for _, permission := range permissions {
		switch permission {
		case acls.EXECVE, acls.FILESYSTEM_READ:
			continue

		default:
			return false, nil
		}
	}
	return true, nil
}

func (self RemappedACLManager) CheckAccessWithArgs(
	permission acls.ACL_PERMISSION, args ...string) (
	bool, error) {
	return false, nil
}

func GetRemappingACLManager(
	config_obj []*config_proto.RemappingConfig) vql_subsystem.ACLManager {
	return RemappedACLManager{}
}
