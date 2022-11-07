package services

import (
	"www.velocidex.com/golang/velociraptor/acls"
	acl_proto "www.velocidex.com/golang/velociraptor/acls/proto"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
)

type ACLManager interface {
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
		permissions ...acls.ACL_PERMISSION) (bool, error)

	CheckAccessWithToken(
		token *acl_proto.ApiClientACL,
		permission acls.ACL_PERMISSION, args ...string) (bool, error)

	GrantRoles(
		config_obj *config_proto.Config,
		principal string,
		roles []string) error
}

func GetACLManager(config_obj *config_proto.Config) (ACLManager, error) {
	org_manager, err := GetOrgManager()
	if err != nil {
		return nil, err
	}

	return org_manager.Services(config_obj.OrgId).ACLManager()
}

// Some helpers for quick access to ACL manager

func GetPolicy(
	config_obj *config_proto.Config,
	principal string) (*acl_proto.ApiClientACL, error) {

	acl_manager, err := GetACLManager(config_obj)
	if err != nil {
		return nil, err
	}

	return acl_manager.GetPolicy(config_obj, principal)
}

func GetEffectivePolicy(
	config_obj *config_proto.Config,
	principal string) (*acl_proto.ApiClientACL, error) {

	acl_manager, err := GetACLManager(config_obj)
	if err != nil {
		return nil, err
	}

	return acl_manager.GetEffectivePolicy(config_obj, principal)
}

func SetPolicy(
	config_obj *config_proto.Config,
	principal string, acl_obj *acl_proto.ApiClientACL) error {
	acl_manager, err := GetACLManager(config_obj)
	if err != nil {
		return err
	}

	return acl_manager.SetPolicy(config_obj, principal, acl_obj)
}

func CheckAccess(
	config_obj *config_proto.Config,
	principal string,
	permissions ...acls.ACL_PERMISSION) (bool, error) {
	acl_manager, err := GetACLManager(config_obj)
	if err != nil {
		return false, err
	}

	return acl_manager.CheckAccess(config_obj, principal, permissions...)
}

func CheckAccessWithToken(
	config_obj *config_proto.Config,
	token *acl_proto.ApiClientACL,
	permission acls.ACL_PERMISSION, args ...string) (bool, error) {
	acl_manager, err := GetACLManager(config_obj)
	if err != nil {
		return false, err
	}

	return acl_manager.CheckAccessWithToken(token, permission, args...)
}

func GrantRoles(
	config_obj *config_proto.Config,
	principal string,
	roles []string) error {

	acl_manager, err := GetACLManager(config_obj)
	if err != nil {
		return err
	}

	return acl_manager.GrantRoles(config_obj, principal, roles)
}
