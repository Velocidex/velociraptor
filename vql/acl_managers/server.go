package acl_managers

import (
	"fmt"
	"sync"

	"www.velocidex.com/golang/velociraptor/acls"
	acl_proto "www.velocidex.com/golang/velociraptor/acls/proto"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	"www.velocidex.com/golang/velociraptor/services"
	"www.velocidex.com/golang/velociraptor/utils"
	vql_subsystem "www.velocidex.com/golang/velociraptor/vql"
)

var (
	lockedDownError = fmt.Errorf("%w: Server locked down", acls.PermissionDenied)
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

// Check against the lockdown token if available. If there is no
// lockdown token (i.e. we are not in lockdown mode) we allow this
// request and further checks are performed.
func (self *ServerACLManager) handleLockdown(
	permissions []acls.ACL_PERMISSION) (bool, error) {
	// Not in lockdown mode, permit access
	if acls.LockdownToken() == nil {
		return true, nil
	}

	// If any of the permissions are denied by the lockdown token then
	// block access.
	for _, perm := range permissions {
		ok, err := services.CheckAccessWithToken(acls.LockdownToken(), perm)
		if err == nil && !ok {
			return false, lockedDownError
		}
	}

	// If we get here all permissions are allowed.
	return true, nil
}

// Token must have *ALL* the specified permissions.
func (self *ServerACLManager) CheckAccess(
	permissions ...acls.ACL_PERMISSION) (bool, error) {

	// Check against the lockdown token and immediately reject
	// permission
	allowed, err := self.handleLockdown(permissions)
	if err != nil || !allowed {
		return false, err
	}

	// If the principal is the super user we allow them everything.
	if self.principal == utils.GetSuperuserName(self.config_obj) {
		return true, nil
	}

	// Check access against the policy
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
