package acl_manager

import (
	"context"
	"errors"
	"fmt"
	"os"
	"sync"
	"time"

	"github.com/Velocidex/ttlcache/v2"
	"www.velocidex.com/golang/velociraptor/acls"
	acl_proto "www.velocidex.com/golang/velociraptor/acls/proto"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	"www.velocidex.com/golang/velociraptor/datastore"
	"www.velocidex.com/golang/velociraptor/paths"
	"www.velocidex.com/golang/velociraptor/services"
	"www.velocidex.com/golang/velociraptor/utils"
)

type ACLManager struct {
	// Cache the effective policy for each principal for 60 sec.
	lru *ttlcache.Cache
}

func NewACLManager(
	ctx context.Context,
	wg *sync.WaitGroup,
	config_obj *config_proto.Config) (*ACLManager, error) {
	result := &ACLManager{
		lru: ttlcache.NewCache(),
	}

	timeout := time.Duration(60 * time.Second)
	if config_obj.Defaults != nil &&
		config_obj.Defaults.AclLruTimeoutSec > 0 {
		timeout = time.Duration(
			config_obj.Defaults.AclLruTimeoutSec) * time.Second
	}

	// ACLs do not typically change that quickly, cache for 60 sec.
	result.lru.SetTTL(timeout)

	return result, nil
}

func (self *ACLManager) GetPolicy(
	config_obj *config_proto.Config,
	principal string) (*acl_proto.ApiClientACL, error) {

	db, err := datastore.GetDB(config_obj)
	if err != nil {
		return nil, err
	}

	acl_obj := &acl_proto.ApiClientACL{}
	user_path_manager := paths.UserPathManager{Name: principal}
	err = db.GetSubject(config_obj, user_path_manager.ACL(), acl_obj)
	if err != nil {
		return nil, err
	}

	return acl_obj, nil
}

// GetEffectivePolicy expands any roles in the policy object to
// produce a simple object.
func (self ACLManager) GetEffectivePolicy(
	config_obj *config_proto.Config,
	principal string) (*acl_proto.ApiClientACL, error) {

	acl_obj_any, err := self.lru.Get(principal)
	if err == nil {
		acl_obj, ok := acl_obj_any.(*acl_proto.ApiClientACL)
		if ok {
			return acl_obj, nil
		}
	}

	db, err := datastore.GetDB(config_obj)
	if err != nil {
		return nil, err
	}

	acl_obj := &acl_proto.ApiClientACL{}

	// The server identity is special - it means the user is an admin.
	if config_obj != nil && config_obj.Client != nil &&
		config_obj.Client.PinnedServerName == principal {
		return &acl_proto.ApiClientACL{SuperUser: true}, nil
	}

	user_path_manager := paths.UserPathManager{Name: principal}
	err = db.GetSubject(config_obj, user_path_manager.ACL(), acl_obj)
	if err != nil {
		return nil, err
	}

	err = acls.GetRolePermissions(config_obj, acl_obj.Roles, acl_obj)
	if err != nil {
		return nil, err
	}

	// Reserved for the server itself - can not be set by normal means.
	acl_obj.SuperUser = false

	self.lru.Set(principal, acl_obj)

	return acl_obj, nil
}

func (self ACLManager) SetPolicy(
	config_obj *config_proto.Config,
	principal string, acl_obj *acl_proto.ApiClientACL) error {

	self.lru.Remove(principal)

	db, err := datastore.GetDB(config_obj)
	if err != nil {
		return err
	}

	user_path_manager := paths.UserPathManager{Name: principal}
	return db.SetSubject(config_obj, user_path_manager.ACL(), acl_obj)
}

func (self ACLManager) CheckAccess(
	config_obj *config_proto.Config,
	principal string,
	permissions ...acls.ACL_PERMISSION) (bool, error) {

	// Internal calls from the server are allowed to do anything.
	if config_obj.Client != nil && principal == config_obj.Client.PinnedServerName {
		return true, nil
	}

	if principal == "" {
		return false, nil
	}

	acl_obj, err := self.GetEffectivePolicy(config_obj, principal)
	if err != nil {
		// A missing ACL means no privs
		if errors.Is(err, os.ErrNotExist) {
			return false, nil
		}
		return false, err
	}

	for _, permission := range permissions {
		ok, err := services.CheckAccessWithToken(acl_obj, permission)
		if !ok || err != nil {
			return ok, err
		}
	}

	return true, nil
}

func (self ACLManager) GrantRoles(
	config_obj *config_proto.Config,
	principal string,
	roles []string) error {
	new_policy := &acl_proto.ApiClientACL{}

	for _, role := range roles {
		if !utils.InString(new_policy.Roles, role) {
			if !acls.ValidateRole(role) {
				return fmt.Errorf("Invalid role %v", role)
			}
			new_policy.Roles = append(new_policy.Roles, role)
		}
	}
	return self.SetPolicy(config_obj, principal, new_policy)
}
