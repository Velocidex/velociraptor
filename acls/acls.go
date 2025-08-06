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
	"strings"
)

type ACL_PERMISSION int

const (
	NO_PERMISSIONS ACL_PERMISSION = iota

	// Issue any query at all.
	ANY_QUERY

	// Publish events to server side queues
	PUBLISH

	// Read results from already run hunts, flows or notebooks.
	READ_RESULTS

	// Can manipulate client labels and metadata.
	LABEL_CLIENT

	// Schedule or cancel new collections on clients.
	COLLECT_CLIENT

	// This is a special custom permission which allows the user to
	// collect "basic" artifacts. For this to work the administrator
	// needs to set the "basic" metadata on the artifact definition.
	COLLECT_BASIC

	// Allows the user to start a hunt
	START_HUNT

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

	// Allowed to manage orgs
	ORG_ADMIN

	// Allows the user to specify a different username for the query() plugin
	IMPERSONATION

	// Allowed to read arbitrary files from the filesystem.
	FILESYSTEM_READ

	// Allowed to create files on the filesystem.
	FILESYSTEM_WRITE

	// Allowed to make network connections
	NETWORK

	// Allowed to collect state information from machines (e.g. pslist()).
	MACHINE_STATE

	// Allowed to create zip files.
	PREPARE_RESULTS

	// Allowed to delete results from the server
	DELETE_RESULTS

	// Allowed raw datastore access
	DATASTORE_ACCESS

	// When adding new permission - update CheckAccess,
	// GetRolePermissions and acl.proto
)

func (self ACL_PERMISSION) MarshalJSON() ([]byte, error) {
	return []byte(fmt.Sprintf("\"%s\"", self.String())), nil
}

func (self ACL_PERMISSION) MarshalYAML() (interface{}, error) {
	return self.String(), nil
}

func (self ACL_PERMISSION) String() string {
	switch self {
	case NO_PERMISSIONS:
		return "NO_PERMISSIONS"
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
	case COLLECT_BASIC:
		return "COLLECT_BASIC"
	case START_HUNT:
		return "START_HUNT"
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
	case ORG_ADMIN:
		return "ORG_ADMIN"
	case IMPERSONATION:
		return "IMPERSONATION"
	case FILESYSTEM_READ:
		return "FILESYSTEM_READ"
	case FILESYSTEM_WRITE:
		return "FILESYSTEM_WRITE"
	case NETWORK:
		return "NETWORK"
	case MACHINE_STATE:
		return "MACHINE_STATE"
	case PREPARE_RESULTS:
		return "PREPARE_RESULTS"
	case DELETE_RESULTS:
		return "DELETE_RESULTS"
	case DATASTORE_ACCESS:
		return "DATASTORE_ACCESS"

	}
	return fmt.Sprintf("%d", self)
}

func GetPermission(name string) ACL_PERMISSION {
	switch strings.ToUpper(name) {
	case "NO_PERMISSIONS":
		return NO_PERMISSIONS
	case "ANY_QUERY":
		return ANY_QUERY
	case "PUBLISH":
		return PUBLISH
	case "READ_RESULTS":
		return READ_RESULTS
	case "LABEL_CLIENT":
		return LABEL_CLIENT
	case "COLLECT_CLIENT":
		return COLLECT_CLIENT
	case "COLLECT_BASIC":
		return COLLECT_BASIC
	case "START_HUNT":
		return START_HUNT
	case "COLLECT_SERVER":
		return COLLECT_SERVER
	case "ARTIFACT_WRITER":
		return ARTIFACT_WRITER
	case "SERVER_ARTIFACT_WRITER":
		return SERVER_ARTIFACT_WRITER
	case "EXECVE":
		return EXECVE
	case "NOTEBOOK_EDITOR":
		return NOTEBOOK_EDITOR
	case "SERVER_ADMIN":
		return SERVER_ADMIN
	case "ORG_ADMIN":
		return ORG_ADMIN
	case "IMPERSONATION":
		return IMPERSONATION
	case "FILESYSTEM_READ":
		return FILESYSTEM_READ
	case "FILESYSTEM_WRITE":
		return FILESYSTEM_WRITE
	case "NETWORK":
		return NETWORK
	case "MACHINE_STATE":
		return MACHINE_STATE
	case "PREPARE_RESULTS":
		return PREPARE_RESULTS
	case "DELETE_RESULTS":
		return DELETE_RESULTS
	case "DATASTORE_ACCESS":
		return DATASTORE_ACCESS

	}
	return NO_PERMISSIONS
}
