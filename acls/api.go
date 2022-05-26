package acls

import (
	"sync"

	acl_proto "www.velocidex.com/golang/velociraptor/acls/proto"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
)

var (
	mu          sync.Mutex
	gACLManager IACLManager = &ACLManager{}
)

func SetACLManager(manager IACLManager) {
	mu.Lock()
	defer mu.Unlock()

	gACLManager = manager
}

type IACLManager interface {
	GetPolicy(
		config_obj *config_proto.Config,
		principal string) (*acl_proto.ApiClientACL, error)

	GetEffectivePolicy(
		config_obj *config_proto.Config,
		principal string) (*acl_proto.ApiClientACL, error)

	SetPolicy(
		config_obj *config_proto.Config,
		principal string, acl_obj *acl_proto.ApiClientACL) error

	CheckAccess(
		config_obj *config_proto.Config,
		principal string,
		permissions ...ACL_PERMISSION) (bool, error)

	CheckAccessWithToken(
		token *acl_proto.ApiClientACL,
		permission ACL_PERMISSION, args ...string) (bool, error)

	GrantRoles(
		config_obj *config_proto.Config,
		principal string,
		roles []string) error
}

func GetPolicy(
	config_obj *config_proto.Config,
	principal string) (*acl_proto.ApiClientACL, error) {
	mu.Lock()
	defer mu.Unlock()
	return gACLManager.GetPolicy(config_obj, principal)
}

func GetEffectivePolicy(
	config_obj *config_proto.Config,
	principal string) (*acl_proto.ApiClientACL, error) {
	mu.Lock()
	defer mu.Unlock()

	return gACLManager.GetEffectivePolicy(config_obj, principal)
}

func SetPolicy(
	config_obj *config_proto.Config,
	principal string, acl_obj *acl_proto.ApiClientACL) error {
	mu.Lock()
	defer mu.Unlock()

	return gACLManager.SetPolicy(config_obj, principal, acl_obj)
}

func CheckAccess(
	config_obj *config_proto.Config,
	principal string,
	permissions ...ACL_PERMISSION) (bool, error) {
	mu.Lock()
	defer mu.Unlock()

	return gACLManager.CheckAccess(config_obj, principal, permissions...)
}

func CheckAccessWithToken(
	token *acl_proto.ApiClientACL,
	permission ACL_PERMISSION, args ...string) (bool, error) {
	mu.Lock()
	defer mu.Unlock()

	return gACLManager.CheckAccessWithToken(token, permission, args...)
}

func GrantRoles(
	config_obj *config_proto.Config,
	principal string,
	roles []string) error {
	return gACLManager.GrantRoles(config_obj, principal, roles)
}
