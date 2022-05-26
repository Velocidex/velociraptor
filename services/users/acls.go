package users

import (
	errors "github.com/pkg/errors"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	datastore "www.velocidex.com/golang/velociraptor/datastore"
	"www.velocidex.com/golang/velociraptor/paths"
	"www.velocidex.com/golang/velociraptor/utils"
)

func (self UserManager) GetPolicy(
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
func (self UserManager) GetEffectivePolicy(
	config_obj *config_proto.Config,
	principal string) (*acl_proto.ApiClientACL, error) {

	db, err := datastore.GetDB(config_obj)
	if err != nil {
		return nil, err
	}

	acl_obj := &acl_proto.ApiClientACL{}

	// The server identity is special - it means the user is an admin.
	if config_obj != nil && config_obj.Client != nil &&
		config_obj.Client.PinnedServerName == principal {
		err = GetRolePermissions(config_obj,
			[]string{"administrator"}, acl_obj)
		if err != nil {
			return nil, err
		}
		return acl_obj, nil
	}

	user_path_manager := paths.UserPathManager{Name: principal}
	err = db.GetSubject(config_obj, user_path_manager.ACL(), acl_obj)
	if err != nil {
		return nil, err
	}

	err = GetRolePermissions(config_obj, acl_obj.Roles, acl_obj)
	if err != nil {
		return nil, err
	}

	return acl_obj, nil
}

func SetPolicy(
	config_obj *config_proto.Config,
	principal string, acl_obj *acl_proto.ApiClientACL) error {

	db, err := datastore.GetDB(config_obj)
	if err != nil {
		return err
	}

	user_path_manager := paths.UserPathManager{Name: principal}
	return db.SetSubject(config_obj, user_path_manager.ACL(), acl_obj)
}

func (self UserManager) CheckAccess(
	config_obj *config_proto.Config,
	principal string,
	permissions ...ACL_PERMISSION) (bool, error) {

	// Internal calls from the server are allowed to do anything.
	if config_obj.Client != nil && principal == config_obj.Client.PinnedServerName {
		return true, nil
	}

	if principal == "" {
		return false, nil
	}

	acl_obj, err := GetEffectivePolicy(config_obj, principal)
	if err != nil {
		return false, err
	}

	for _, permission := range permissions {
		ok, err := CheckAccessWithToken(acl_obj, permission)
		if !ok || err != nil {
			return ok, err
		}
	}

	return true, nil
}

func (self UserManager) CheckAccessWithToken(
	token *acl_proto.ApiClientACL,
	permission ACL_PERMISSION, args ...string) (bool, error) {

	// Requested permission
	switch permission {
	case ALL_QUERY:
		return token.AllQuery, nil

	case ANY_QUERY:
		return token.AnyQuery, nil

	case PUBLISH:
		if len(args) == 1 {
			for _, allowed_queue := range token.PublishQueues {
				if allowed_queue == args[0] {
					return true, nil
				}

			}
		}

	case READ_RESULTS:
		return token.ReadResults, nil

	case LABEL_CLIENT:
		return token.LabelClients, nil

	case COLLECT_CLIENT:
		return token.CollectClient, nil

	case COLLECT_SERVER:
		return token.CollectServer, nil

	case ARTIFACT_WRITER:
		return token.ArtifactWriter, nil

	case SERVER_ARTIFACT_WRITER:
		return token.ServerArtifactWriter, nil

	case EXECVE:
		return token.Execve, nil

	case NOTEBOOK_EDITOR:
		return token.NotebookEditor, nil

	case SERVER_ADMIN:
		return token.ServerAdmin, nil

	case FILESYSTEM_READ:
		return token.FilesystemRead, nil

	case FILESYSTEM_WRITE:
		return token.FilesystemWrite, nil

	case MACHINE_STATE:
		return token.MachineState, nil

	case PREPARE_RESULTS:
		return token.PrepareResults, nil

	case DATASTORE_ACCESS:
		return token.DatastoreAccess, nil

	}

	return false, nil
}

func (self UserManager) GrantRoles(
	config_obj *config_proto.Config,
	principal string,
	roles []string) error {
	new_policy := &acl_proto.ApiClientACL{}

	for _, role := range roles {
		if !utils.InString(new_policy.Roles, role) {
			if !ValidateRole(role) {
				return errors.Errorf("Invalid role %v", role)
			}
			new_policy.Roles = append(new_policy.Roles, role)
		}
	}
	return self.SetPolicy(config_obj, principal, new_policy)
}
