package acl_managers

import (
	"errors"
	"sync"

	"www.velocidex.com/golang/velociraptor/acls"
	acl_proto "www.velocidex.com/golang/velociraptor/acls/proto"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	"www.velocidex.com/golang/velociraptor/services"
	vql_subsystem "www.velocidex.com/golang/velociraptor/vql"
)

var (
	notLockedDownError = errors.New("PERMISSION_DENIED: Server locked down")
)

// ServerACLManager is used when running server side VQL to control
// ACLs on various VQL plugins.
type ServerACLManager struct {
	principal  string
	config_obj *config_proto.Config

	// Cache principal's token for each org_id
	mu         sync.Mutex
	TokenCache map[string]*acl_proto.ApiClientACL
}

func (self *ServerACLManager) GetPrincipal() string {
	return self.principal
}

func (self *ServerACLManager) handleLockdown(
	permissions []acls.ACL_PERMISSION) (bool, error) {
	if acls.LockdownToken() == nil {
		return false, nil
	}

	for _, perm := range permissions {
		ok, err := services.CheckAccessWithToken(acls.LockdownToken(), perm)
		if err == nil && ok {
			return false, notLockedDownError
		}
	}
	return false, nil
}

// Token must have *ALL* the specified permissions.
func (self *ServerACLManager) CheckAccess(
	permissions ...acls.ACL_PERMISSION) (bool, error) {

	// If we are in lockdown, immediately reject permission
	ok, err := self.handleLockdown(permissions)
	if err != nil {
		return ok, err
	}

	policy, err := self.getPolicyInOrg(self.config_obj.OrgId)
	if err != nil {
		return false, err
	}

	for _, permission := range permissions {
		ok, err := services.CheckAccessWithToken(policy, permission)
		if !ok || err != nil {
			return ok, err
		}
	}

	return true, nil
}

func (self *ServerACLManager) getPolicyInOrg(org_id string) (*acl_proto.ApiClientACL, error) {
	self.mu.Lock()
	policy, pres := self.TokenCache[org_id]
	self.mu.Unlock()
	if pres && policy != nil {
		return policy, nil
	}

	org_manager, err := services.GetOrgManager()
	if err != nil {
		return nil, err
	}

	org_config_obj, err := org_manager.GetOrgConfig(org_id)
	if err != nil {
		return nil, err
	}

	policy, err = services.GetEffectivePolicy(org_config_obj, self.principal)
	if err != nil {
		return nil, err
	}

	self.mu.Lock()
	self.TokenCache[org_id] = policy
	self.mu.Unlock()

	return policy, nil
}

func (self *ServerACLManager) CheckAccessInOrg(
	org_id string, permissions ...acls.ACL_PERMISSION) (bool, error) {
	policy, err := self.getPolicyInOrg(org_id)
	if err != nil {
		return false, err
	}
	for _, permission := range permissions {
		ok, err := services.CheckAccessWithToken(policy, permission)
		if !ok || err != nil {
			return ok, err
		}
	}

	return true, nil
}

func (self *ServerACLManager) CheckAccessWithArgs(
	permission acls.ACL_PERMISSION, args ...string) (bool, error) {
	policy, err := self.getPolicyInOrg(self.config_obj.OrgId)
	if err != nil {
		return false, err
	}

	return services.CheckAccessWithToken(policy, permission, args...)
}

func NewServerACLManager(
	config_obj *config_proto.Config,
	principal string) vql_subsystem.ACLManager {
	return &ServerACLManager{
		principal:  principal,
		config_obj: config_obj,
		TokenCache: make(map[string]*acl_proto.ApiClientACL),
	}
}
