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

Note that the interaction of different permission may be used to
bypass the RBACS - for example:

1. Given SERVER_ARTIFACT_WRITER and COLLECT_SERVER allows one to write
   an artifact which runs with full permissions on the server
   (i.e. arbitrary execution).

2. Given ARTIFACT_WRITER and COLLECT_CLIENT give one full control over
   endpoints.

Tips:

- Since Velociraptor is a VQL based system writing arbitrary VQL can
  provide the user with a lot of power. Server side VQL typically runs
  with full privileges so being able to add server side artifacts is
  equivalent to admin access.


*/

import (
	"fmt"
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

	// Add or edit custom artifacts that run on endpoints.
	ARTIFACT_WRITER

	// Add or edit custom artifacts that run on the server.
	SERVER_ARTIFACT_WRITER

	// Allowed to run the execve command.
	EXECVE

	// Allowed to change notebooks and cells
	NOTEBOOK_EDITOR

	// Allowed to manage server configuration.
	SERVER_ADMIN

	// Allowed to read arbitrary files from the filesystem.
	FILESYSTEM_READ

	// Allowed to create files on the filesystem.
	FILESYSTEM_WRITE

	// Allowed to collect state information from machines (e.g. pslist()).
	MACHINE_STATE

	// When adding new permission - update CheckAccess,
	// GetRolePermissions and acl.proto
)

func (self ACL_PERMISSION) String() string {
	switch self {
	case ALL_QUERY:
		return "ALL_QUERY"
	case ANY_QUERY:
		return "ANY_QUERY"
	case PUBLISH:
		return "PUBLISH"
	case READ_RESULTS:
		return "READ_RESULTS"
	case LABEL_CLIENT:
		return "LABEL_CLIENT"
	case COLLECT_CLIENT:
		return "COLLECT_CLIENT"
	case COLLECT_SERVER:
		return "COLLECT_SERVER"
	case ARTIFACT_WRITER:
		return "ARTIFACT_WRITER"
	case SERVER_ARTIFACT_WRITER:
		return "SERVER_ARTIFACT_WRITER"
	case EXECVE:
		return "EXECVE"
	case NOTEBOOK_EDITOR:
		return "NOTEBOOK_EDITOR"
	case SERVER_ADMIN:
		return "SERVER_ADMIN"
	case FILESYSTEM_READ:
		return "FILESYSTEM_READ"
	case FILESYSTEM_WRITE:
		return "FILESYSTEM_WRITE"
	case MACHINE_STATE:
		return "MACHINE_STATE"
	}
	return fmt.Sprintf("%d", self)
}

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

	return CheckAccessWithToken(acl_obj, permission, args...)
}

func CheckAccessWithToken(
	token *acl_proto.ApiClientACL,
	permission ACL_PERMISSION, args ...string) (bool, error) {

	// Requested permission
	switch permission {
	case ANY_QUERY:
		// Principal is allowed all queries.
		return token.AllQuery, nil

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

	}

	return false, nil
}
