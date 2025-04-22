package paths

import (
	"www.velocidex.com/golang/velociraptor/file_store/api"
	"www.velocidex.com/golang/velociraptor/file_store/path_specs"
)

var (
	// This is the old index it will be automatically upgraded to the
	// new index.
	CLIENT_INDEX_URN_DEPRECATED = path_specs.NewSafeDatastorePath("client_index").
					SetType(api.PATH_TYPE_DATASTORE_PROTO).
					SetTag("ClientIndex")

	// The GUI uses this as the client index.
	CLIENT_INDEX_URN = path_specs.NewUnsafeDatastorePath("client_idx").
				SetType(api.PATH_TYPE_DATASTORE_PROTO).
				SetTag("ClientIndex")

	FLOWS_JOUNRNAL = path_specs.NewSafeFilestorePath("flows_journal").
			SetType(api.PATH_TYPE_FILESTORE_JSON)

	// An index of all the hunts and clients.
	HUNT_INDEX = path_specs.NewSafeDatastorePath("hunt_index").
			SetType(api.PATH_TYPE_DATASTORE_PROTO)

	NOTEBOOK_INDEX = path_specs.NewSafeDatastorePath("notebook_index").
			SetType(api.PATH_TYPE_DATASTORE_PROTO)

	NOTEBOOK_ROOT = path_specs.NewSafeDatastorePath("notebooks").
			SetType(api.PATH_TYPE_DATASTORE_JSON)

	BACKUPS_ROOT = path_specs.NewSafeFilestorePath("backups").
			SetType(api.PATH_TYPE_FILESTORE_ANY)

	DOWNLOADS_ROOT = path_specs.NewUnsafeFilestorePath("downloads").
			SetType(api.PATH_TYPE_FILESTORE_DOWNLOAD_ZIP)

	CLIENTS_ROOT = path_specs.NewUnsafeDatastorePath("clients").
			SetType(api.PATH_TYPE_DATASTORE_PROTO)

	// Stores a snapshot of all client records
	CLIENTS_INFO_SNAPSHOT = path_specs.NewUnsafeFilestorePath(
		"client_info", "snapshot").
		SetType(api.PATH_TYPE_FILESTORE_JSON)

	CONFIG_ROOT = path_specs.NewSafeDatastorePath("config").
			SetType(api.PATH_TYPE_DATASTORE_JSON)

	HUNTS_ROOT = path_specs.NewSafeDatastorePath("hunts").
			SetType(api.PATH_TYPE_DATASTORE_PROTO)

	USERS_ROOT = path_specs.NewUnsafeDatastorePath("users").
			SetType(api.PATH_TYPE_DATASTORE_JSON)

	ACL_ROOT = path_specs.NewUnsafeDatastorePath("acl").
			SetType(api.PATH_TYPE_DATASTORE_JSON)

	ORGS_ROOT = path_specs.NewSafeDatastorePath("orgs").
			SetType(api.PATH_TYPE_DATASTORE_JSON)

	// The public directory is exported without authentication and
	// is used to distribute the client binaries.
	PUBLIC_ROOT = path_specs.NewUnsafeFilestorePath("public").
			SetType(api.PATH_TYPE_FILESTORE_ANY)

	TEMP_ROOT = path_specs.NewUnsafeFilestorePath("temp").
			SetType(api.PATH_TYPE_FILESTORE_ANY)

	// Timelines
	TIMELINE_URN = path_specs.NewSafeDatastorePath("timelines").
			SetType(api.PATH_TYPE_DATASTORE_JSON)

	SERVER_MONITORING_ROOT = path_specs.NewSafeFilestorePath(
		"server_artifacts")
	SERVER_MONITORING_LOGS_ROOT = path_specs.NewSafeFilestorePath(
		"server_artifact_logs")

	// Filestore paths for artifacts must begin with this prefix.
	ARTIFACT_DEFINITION_PREFIX = path_specs.NewSafeFilestorePath(
		"artifact_definitions").
		SetType(api.PATH_TYPE_FILESTORE_YAML)

	// These store configuration for the server and client
	// monitoring artifacts.
	ServerMonitoringFlowURN = path_specs.NewSafeDatastorePath("config",
		"server_monitoring").SetType(api.PATH_TYPE_DATASTORE_JSON)

	ClientMonitoringFlowURN = path_specs.NewSafeDatastorePath(
		"config", "client_monitoring").SetType(api.PATH_TYPE_DATASTORE_JSON)

	ThirdPartyInventory = path_specs.NewSafeDatastorePath(
		"config", "inventory").SetType(api.PATH_TYPE_DATASTORE_JSON)
)
