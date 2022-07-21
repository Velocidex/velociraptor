package acls

import (
	"errors"
	"strings"

	acl_proto "www.velocidex.com/golang/velociraptor/acls/proto"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
)

func ValidateRole(role string) bool {
	switch role {
	case "org_admin", "administrator", "reader", "analyst", "investigator", "artifact_writer", "api":
		return true
	}

	return false
}

func SetTokenPermission(
	token *acl_proto.ApiClientACL, permissions ...string) error {
	for _, perm := range permissions {
		switch strings.ToUpper(perm) {
		case "ALL_QUERY":
			token.AllQuery = true
		case "ANY_QUERY":
			token.AnyQuery = true
		case "READ_RESULTS":
			token.ReadResults = true
		case "LABEL_CLIENT":
			token.LabelClients = true
		case "COLLECT_CLIENT":
			token.CollectClient = true
		case "COLLECT_SERVER":
			token.CollectServer = true
		case "ARTIFACT_WRITER":
			token.ArtifactWriter = true
		case "SERVER_ARTIFACT_WRITER":
			token.ServerArtifactWriter = true
		case "EXECVE":
			token.Execve = true
		case "NOTEBOOK_EDITOR":
			token.NotebookEditor = true
		case "SERVER_ADMIN":
			token.ServerAdmin = true
		case "ORG_ADMIN":
			token.OrgAdmin = true
		case "IMPERSONATION":
			token.Impersonation = true
		case "FILESYSTEM_READ":
			token.FilesystemRead = true
		case "FILESYSTEM_WRITE":
			token.FilesystemWrite = true
		case "MACHINE_STATE":
			token.MachineState = true
		case "PREPARE_RESULTS":
			token.PrepareResults = true
		case "DATASTORE_ACCESS":
			token.DatastoreAccess = true

		default:
			return errors.New("Unknown permission")
		}
	}

	return nil
}

func GetRolePermissions(
	config_obj *config_proto.Config,
	roles []string, result *acl_proto.ApiClientACL) error {

	for _, role := range roles {
		switch role {

		case "org_admin":
			result.OrgAdmin = true

		// Admins get all query access
		case "administrator":
			result.AllQuery = true
			result.AnyQuery = true
			result.ReadResults = true
			result.Impersonation = true
			result.LabelClients = true
			result.CollectClient = true
			result.CollectServer = true
			result.ArtifactWriter = true
			result.ServerArtifactWriter = true
			result.Execve = true
			result.NotebookEditor = true
			result.ServerAdmin = true
			result.FilesystemRead = true
			result.FilesystemWrite = true
			result.MachineState = true
			result.PrepareResults = true

			// An administrator for the root org is allowed to
			// manipulate orgs.
			if config_obj != nil && config_obj.OrgId == "" {
				result.OrgAdmin = true
			}

			// Readers can view results but not edit or
			// modify anything.
		case "reader":
			result.ReadResults = true

			// An API client can read results
			// (e.g watch_monitoring)
		case "api":
			result.AnyQuery = true
			result.ReadResults = true

			// Analysts can post process results using
			// notebooks. They can issue new VQL that
			// looks at existing results but not VQL that
			// collects new data or runs anything.
		case "analyst":
			result.ReadResults = true
			result.NotebookEditor = true
			result.LabelClients = true
			result.AnyQuery = true
			result.PrepareResults = true

			// Investigators are like analysts but can
			// also issue new collections from endpoints.
		case "investigator":
			result.ReadResults = true
			result.NotebookEditor = true
			result.CollectClient = true
			result.LabelClients = true
			result.AnyQuery = true
			result.PrepareResults = true

			// Artifact writers are allowed to edit and
			// create artifacts. NOTE This role is akin to
			// administrator, it allows root on endpoints!
		case "artifact_writer":
			result.ArtifactWriter = true

		default:
			return errors.New("Unknown role")
		}
	}

	result.Roles = nil
	return nil
}
