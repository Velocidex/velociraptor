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
	token *acl_proto.ApiClientACL,
	permission acls.ACL_PERMISSION, args ...string) (bool, error) {

	// The super user can do everything.
	if token.SuperUser {
		return true, nil
	}

	// Requested permission
	switch permission {

	case acls.ANY_QUERY:
		return token.AnyQuery, nil

	case acls.PUBLISH:
		if len(args) == 1 {
			for _, allowed_queue := range token.PublishQueues {
				if allowed_queue == args[0] {
					return true, nil
				}

			}
		}

	case acls.READ_RESULTS:
		return token.ReadResults, nil

	case acls.LABEL_CLIENT:
		return token.LabelClients, nil

	case acls.COLLECT_CLIENT:
		return token.CollectClient, nil

	case acls.COLLECT_BASIC:
		return token.CollectBasic, nil

	case acls.START_HUNT:
		return token.StartHunt, nil

	case acls.COLLECT_SERVER:
		return token.CollectServer, nil

	case acls.ARTIFACT_WRITER:
		return token.ArtifactWriter, nil

	case acls.SERVER_ARTIFACT_WRITER:
		return token.ServerArtifactWriter, nil

	case acls.EXECVE:
		return token.Execve, nil

	case acls.NOTEBOOK_EDITOR:
		return token.NotebookEditor, nil

	case acls.SERVER_ADMIN:
		return token.ServerAdmin, nil

	case acls.ORG_ADMIN:
		return token.OrgAdmin, nil

	case acls.IMPERSONATION:
		return token.Impersonation, nil

	case acls.FILESYSTEM_READ:
		return token.FilesystemRead, nil

	case acls.NETWORK:
		return token.Network, nil

	case acls.FILESYSTEM_WRITE:
		return token.FilesystemWrite, nil

	case acls.MACHINE_STATE:
		return token.MachineState, nil

	case acls.PREPARE_RESULTS:
		return token.PrepareResults, nil

	case acls.DELETE_RESULTS:
		return token.DeleteResults, nil

	case acls.DATASTORE_ACCESS:
		return token.DatastoreAccess, nil
	}

	return false, nil
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
