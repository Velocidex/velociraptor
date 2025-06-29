package acl_manager

import (
	"context"
	"errors"
	"fmt"
	"os"
	"sync"
	"time"

	"google.golang.org/protobuf/proto"
	"www.velocidex.com/golang/velociraptor/acls"
	acl_proto "www.velocidex.com/golang/velociraptor/acls/proto"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	"www.velocidex.com/golang/velociraptor/datastore"
	"www.velocidex.com/golang/velociraptor/logging"
	"www.velocidex.com/golang/velociraptor/paths"
	"www.velocidex.com/golang/velociraptor/services"
	"www.velocidex.com/golang/velociraptor/utils"
)

var (
	notLockedDownError = errors.New("PERMISSION_DENIED: Server locked down")
)

type _CachedACLObject struct {
	policy   *acl_proto.ApiClientACL
	username string
}

type ACLManager struct {
	mu sync.Mutex

	// Cache the effective policy for each principal in this org for 60 sec.
	cache map[string]*_CachedACLObject
}

func (self *ACLManager) reloadCache(
	ctx context.Context, config_obj *config_proto.Config) error {
	cache := make(map[string]*_CachedACLObject)

	// We can not load ACLs from a datastore which is not
	// configured. This is used in tools etc. It will just result in
	// all permission denied errors as the ACL cache will be empty so
	// we actually create a more restricted environment.
	if config_obj.Datastore == nil {
		return nil
	}

	db, err := datastore.GetDB(config_obj)
	if err != nil {
		return err
	}

	children, err := db.ListChildren(config_obj, paths.ACL_ROOT)
	if err != nil {
		return err
	}

	for _, child := range children {
		if child.IsDir() {
			continue
		}

		username := child.Base()
		policy := &acl_proto.ApiClientACL{}
		err = db.GetSubject(config_obj, child, policy)
		if err != nil {
			continue
		}

		// Detect ACL files with multiple casing - we reject one to
		// avoid ACL confusion. This should not occur in normal
		// operation!
		lower_user_name := utils.ToLower(username)
		old_record, pres := cache[lower_user_name]
		if pres && old_record.username != username {
			logger := logging.GetLogger(config_obj, &logging.FrontendComponent)
			logger.Error("<red>ACLManager</>: In Org %v multiple casing detected for ACLs for %v, will use record for %v.",
				utils.GetOrgId(config_obj), username, old_record.username)
			continue
		}

		cache[lower_user_name] = &_CachedACLObject{
			policy:   policy,
			username: username,
		}
	}

	// Swap the new cache quickly
	self.mu.Lock()
	self.cache = cache
	self.mu.Unlock()

	return nil
}

func NewACLManager(
	ctx context.Context,
	wg *sync.WaitGroup,
	config_obj *config_proto.Config) (*ACLManager, error) {
	self := &ACLManager{}

	refresh_duration := time.Duration(300 * time.Second)
	if config_obj.Defaults != nil &&
		config_obj.Defaults.AclLruTimeoutSec > 0 {
		refresh_duration = time.Duration(
			config_obj.Defaults.AclLruTimeoutSec) * time.Second
	}

	err := self.reloadCache(ctx, config_obj)
	if err != nil {
		return nil, err
	}

	backups, err := services.GetBackupService(config_obj)
	if err == nil {
		backups.Register(&ACLBackupProvider{
			config_obj: config_obj,
			manager:    self,
		})
	}

	wg.Add(1)
	go func() {
		defer wg.Done()

		for {
			select {
			case <-ctx.Done():
				return
			case <-time.After(utils.Jitter(refresh_duration)):
				err := self.reloadCache(ctx, config_obj)
				if err != nil {
					logger := logging.GetLogger(config_obj, &logging.FrontendComponent)
					logger.Error("ACLManager reloadCache: %v", err)
				}
			}
		}

	}()

	return self, nil
}

func (self *ACLManager) GetPolicy(
	config_obj *config_proto.Config,
	principal string) (*acl_proto.ApiClientACL, error) {

	self.mu.Lock()
	defer self.mu.Unlock()

	lower_user_name := utils.ToLower(principal)
	cache, pres := self.cache[lower_user_name]
	if pres {
		return proto.Clone(cache.policy).(*acl_proto.ApiClientACL), nil
	}
	return nil, utils.NotFoundError
}

// GetEffectivePolicy expands any roles in the policy object to
// produce a simple object.
func (self *ACLManager) GetEffectivePolicy(
	config_obj *config_proto.Config,
	principal string) (*acl_proto.ApiClientACL, error) {

	// The server identity is special - it means the user is an admin.
	if principal == utils.GetSuperuserName(config_obj) {
		return &acl_proto.ApiClientACL{SuperUser: true}, nil
	}

	policy, err := self.GetPolicy(config_obj, principal)
	if err != nil {
		return nil, err
	}

	err = acls.GetRolePermissions(config_obj, policy.Roles, policy)
	if err != nil {
		return nil, err
	}

	// Reserved for the server itself - can not be set by normal means.
	policy.SuperUser = false

	return policy, nil
}

func (self *ACLManager) SetPolicy(
	config_obj *config_proto.Config,
	principal string, acl_obj *acl_proto.ApiClientACL) error {

	self.mu.Lock()
	defer self.mu.Unlock()

	// Normalize the username casing.
	lower_user_name := utils.ToLower(principal)
	cache, pres := self.cache[lower_user_name]
	if pres {
		principal = cache.username
	}

	self.cache[lower_user_name] = &_CachedACLObject{
		policy:   proto.Clone(acl_obj).(*acl_proto.ApiClientACL),
		username: principal,
	}

	db, err := datastore.GetDB(config_obj)
	if err != nil {
		return err
	}

	// Store the ACL with the original user casing.
	user_path_manager := paths.UserPathManager{Name: principal}
	return db.SetSubject(config_obj, user_path_manager.ACL(), acl_obj)
}

func (self *ACLManager) handleLockdown(
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

func (self *ACLManager) CheckAccess(
	config_obj *config_proto.Config,
	principal string,
	permissions ...acls.ACL_PERMISSION) (bool, error) {

	// If we are in lockdown, immediately reject permission
	ok, err := self.handleLockdown(permissions)
	if err != nil {
		return ok, err
	}

	// Internal calls from the server are allowed to do anything.
	if principal == utils.GetSuperuserName(config_obj) {
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

func (self *ACLManager) GrantRoles(
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
