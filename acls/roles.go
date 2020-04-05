package acls

import (
	"errors"

	acl_proto "www.velocidex.com/golang/velociraptor/acls/proto"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
)

func ValidateRole(role string) bool {
	switch role {
	case "administrator", "reader", "analyst", "investigator", "artifact_writer", "api":
		return true
	}

	return false
}

func GetRolePermissions(
	config_obj *config_proto.Config,
	roles []string, result *acl_proto.ApiClientACL) error {

	for _, role := range roles {
		switch role {

		// Admins get all query access
		case "administrator":
			result.AllQuery = true
			result.AnyQuery = true
			result.ReadResults = true
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
