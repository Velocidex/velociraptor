package acls

/*

ACLs enforce access to Velociraptor.

API Clients are created by using the "config api_client" command -
this generates a certificate with a common name. This common name is
associated with the particular program which uses the api_client
certificate.

A GUI user is created via the "user add" command.

Both GUI users and API Clients are considered "Principals" and have an
ACL object attached to them. You can grant a principal specific
permissions of any number of roles using the "acl grant" command.

$ velociraptor acl grant mike@velocidex.com --role administrator

## What are permissions?

Various actions within the Velociraptor server require permissions to
run. To figure out if the principal that is running the action is
allowed, the code checks the principal against an ACL_PERMISSION
below. ACL_PERMISSION represents what the code wants to do.

A Role is a collection of permissions that are granted to anyone in
that role.

*/

import (
	"path"

	acl_proto "www.velocidex.com/golang/velociraptor/acls/proto"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	"www.velocidex.com/golang/velociraptor/datastore"
)

type ACL_PERMISSION int

const (
	// Issue all queries without restriction
	ALL_QUERY ACL_PERMISSION = iota

	// Issue any query at all (ALL_QUERY implies ANY_QUERY).
	ANY_QUERY

	// Publish events to server side queues
	PUBLISH

	// Read results from already run hunts, flows or notebooks.
	READ_RESULTS

	// Can manipulate client labels.
	LABEL_CLIENT

	// Schedule or cancel new collections on clients.
	COLLECT_CLIENT

	// Schedule new artifact collections on velociraptor servers.
	COLLECT_SERVER

	// Add or edit custom artifacts
	ARTIFACT_WRITER

	// Allowed to run the execve command.
	EXECVE

	// Allowed to change notebooks and cells
	NOTEBOOK_EDITOR

	// Allowed to manage server configuration.
	SERVER_ADMIN

	// When adding new permission - update CheckAccess,
	// GetRolePermissions and acl.proto
)

func GetPolicy(
	config_obj *config_proto.Config,
	principal string) (*acl_proto.ApiClientACL, error) {

	db, err := datastore.GetDB(config_obj)
	if err != nil {
		return nil, err
	}

	acl_obj := &acl_proto.ApiClientACL{}
	err = db.GetSubject(config_obj,
		path.Join("acl", principal+".json"), acl_obj)
	if err != nil {
		return nil, err
	}

	return acl_obj, nil
}

// GetEffectivePolicy expands any roles in the policy object to
// produce a simple object.
func GetEffectivePolicy(
	config_obj *config_proto.Config,
	principal string) (*acl_proto.ApiClientACL, error) {

	db, err := datastore.GetDB(config_obj)
	if err != nil {
		return nil, err
	}

	acl_obj := &acl_proto.ApiClientACL{}
	err = db.GetSubject(config_obj,
		path.Join("acl", principal+".json"), acl_obj)
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

	return db.SetSubject(config_obj,
		path.Join("acl", principal+".json"), acl_obj)
}

func CheckAccess(
	config_obj *config_proto.Config,
	principal string,
	permission ACL_PERMISSION, args ...string) (bool, error) {

	// Internal calls from the server are allowed to do anything.
	if principal == config_obj.Client.PinnedServerName {
		return true, nil
	}

	if principal == "" {
		return false, nil
	}

	acl_obj, err := GetEffectivePolicy(config_obj, principal)
	if err != nil {
		return false, err
	}

	// Requested permission
	switch permission {
	case ANY_QUERY:
		// Principal is allowed all queries.
		return acl_obj.AllQuery, nil

	case PUBLISH:
		if len(args) == 1 {
			for _, allowed_queue := range acl_obj.PublishQueues {
				if allowed_queue == args[0] {
					return true, nil
				}

			}
		}

	case READ_RESULTS:
		return acl_obj.ReadResults, nil

	case LABEL_CLIENT:
		return acl_obj.LabelClients, nil

	case COLLECT_CLIENT:
		return acl_obj.CollectClient, nil

	case COLLECT_SERVER:
		return acl_obj.CollectServer, nil

	case ARTIFACT_WRITER:
		return acl_obj.ArtifactWriter, nil

	case EXECVE:
		return acl_obj.Execve, nil

	case NOTEBOOK_EDITOR:
		return acl_obj.NotebookEditor, nil

	case SERVER_ADMIN:
		return acl_obj.ServerAdmin, nil
	}

	return false, nil
}
