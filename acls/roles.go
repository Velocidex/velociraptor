package acls

import (
	"errors"
	"strings"

	acl_proto "www.velocidex.com/golang/velociraptor/acls/proto"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	"www.velocidex.com/golang/velociraptor/utils"
)

var (
	ALL_ROLES = []string{"org_admin", "administrator", "reader",
		"analyst", "investigator",
		"artifact_writer", "api"}

	ALL_PERMISSIONS = []string{
		"ANY_QUERY",
		"READ_RESULTS",
		"LABEL_CLIENT",
		"COLLECT_CLIENT",
		"COLLECT_BASIC",
		"START_HUNT",
		"COLLECT_SERVER",
		"ARTIFACT_WRITER",
		"SERVER_ARTIFACT_WRITER",
		"EXECVE",
		"NOTEBOOK_EDITOR",
		"SERVER_ADMIN",
		"ORG_ADMIN",
		"IMPERSONATION",
		"FILESYSTEM_READ",
		"FILESYSTEM_WRITE",
		"NETWORK",
		"MACHINE_STATE",
		"PREPARE_RESULTS",
		"DELETE_RESULTS",
		"DATASTORE_ACCESS",
	}
)

func ValidateRole(role string) bool {
	return utils.InString(ALL_ROLES, role)
}

func DescribePermissions(token *acl_proto.ApiClientACL) []string {
	result := []string{}
	if token.AnyQuery {
		result = append(result, "ANY_QUERY")
	}
	if token.ReadResults {
		result = append(result, "READ_RESULTS")
	}
	if token.LabelClients {
		result = append(result, "LABEL_CLIENT")
	}
	if token.CollectClient {
		result = append(result, "COLLECT_CLIENT")
	}
	if token.CollectBasic {
		result = append(result, "COLLECT_BASIC")
	}
	if token.StartHunt {
		result = append(result, "START_HUNT")
	}
	if token.CollectServer {
		result = append(result, "COLLECT_SERVER")
	}
	if token.ArtifactWriter {
		result = append(result, "ARTIFACT_WRITER")
	}
	if token.ServerArtifactWriter {
		result = append(result, "SERVER_ARTIFACT_WRITER")
	}
	if token.Execve {
		result = append(result, "EXECVE")
	}
	if token.NotebookEditor {
		result = append(result, "NOTEBOOK_EDITOR")
	}
	if token.ServerAdmin {
		result = append(result, "SERVER_ADMIN")
	}
	if token.OrgAdmin {
		result = append(result, "ORG_ADMIN")
	}
	if token.Impersonation {
		result = append(result, "IMPERSONATION")
	}
	if token.FilesystemRead {
		result = append(result, "FILESYSTEM_READ")
	}

	if token.FilesystemWrite {
		result = append(result, "FILESYSTEM_WRITE")
	}

	if token.Network {
		result = append(result, "NETWORK")
	}

	if token.MachineState {
		result = append(result, "MACHINE_STATE")
	}

	if token.PrepareResults {
		result = append(result, "PREPARE_RESULTS")
	}

	if token.DeleteResults {
		result = append(result, "DELETE_RESULTS")
	}

	if token.DatastoreAccess {
		result = append(result, "DATASTORE_ACCESS")
	}

	return result
}

func SetTokenPermission(
	token *acl_proto.ApiClientACL, permissions ...string) error {
	for _, perm := range permissions {
		switch strings.ToUpper(perm) {
		case "ANY_QUERY":
			token.AnyQuery = true
		case "READ_RESULTS":
			token.ReadResults = true
		case "LABEL_CLIENT":
			token.LabelClients = true
		case "COLLECT_CLIENT":
			token.CollectClient = true
		case "COLLECT_BASIC":
			token.CollectBasic = true
		case "START_HUNT":
			token.StartHunt = true
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
		case "NETWORK":
			token.Network = true
		case "MACHINE_STATE":
			token.MachineState = true
		case "PREPARE_RESULTS":
			token.PrepareResults = true
		case "DELETE_RESULTS":
			token.DeleteResults = true
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
			result.AnyQuery = true
			result.ReadResults = true
			result.Impersonation = true
			result.LabelClients = true
			result.CollectClient = true
			result.CollectBasic = true
			result.StartHunt = true
			result.CollectServer = true
			result.ArtifactWriter = true
			result.ServerArtifactWriter = true
			result.Execve = true
			result.NotebookEditor = true
			result.ServerAdmin = true
			result.FilesystemRead = true
			result.FilesystemWrite = true
			result.Network = true
			result.MachineState = true
			result.PrepareResults = true
			result.DeleteResults = true

			// An administrator for the root org is allowed to
			// manipulate orgs.
			if config_obj != nil && utils.IsRootOrg(config_obj.OrgId) {
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
			result.StartHunt = true
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
