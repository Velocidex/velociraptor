package paths

import "www.velocidex.com/golang/velociraptor/file_store/api"

var (
	// The GUI uses this as the client index.
	CLIENT_INDEX_URN = api.NewSafeDatastorePath("client_index").
				SetType(api.PATH_TYPE_DATASTORE_PROTO)

	// An index of all the hunts and clients.
	HUNT_INDEX = api.NewSafeDatastorePath("hunt_index").
			SetType(api.PATH_TYPE_DATASTORE_PROTO)

	NOTEBOOK_INDEX = api.NewSafeDatastorePath("notebook_index").
			SetType(api.PATH_TYPE_DATASTORE_PROTO)

	NOTEBOOK_ROOT = api.NewSafeDatastorePath("notebooks").
			SetType(api.PATH_TYPE_DATASTORE_JSON)

	DOWNLOADS_ROOT = api.NewSafeDatastorePath("downloads").
			SetType(api.PATH_TYPE_FILESTORE_DOWNLOAD_ZIP)

	CLIENTS_ROOT = api.NewSafeDatastorePath("clients").
			SetType(api.PATH_TYPE_DATASTORE_PROTO)

	CONFIG_ROOT = api.NewSafeDatastorePath("config").
			SetType(api.PATH_TYPE_DATASTORE_JSON)

	HUNTS_ROOT = api.NewSafeDatastorePath("hunts").
			SetType(api.PATH_TYPE_DATASTORE_PROTO)

	USERS_ROOT = api.NewUnsafeDatastorePath("users").
			SetType(api.PATH_TYPE_DATASTORE_JSON)

	ACL_ROOT = api.NewUnsafeDatastorePath("acl").
			SetType(api.PATH_TYPE_DATASTORE_JSON)

	// The public directory is exported without authentication and
	// is used to distribute the client binaries.
	PUBLIC_ROOT = api.NewUnsafeDatastorePath("public").
			SetType(api.PATH_TYPE_FILESTORE_ANY)

	// Timelines
	TIMELINE_URN = api.NewSafeDatastorePath("timelines").
			SetType(api.PATH_TYPE_DATASTORE_JSON)

	// Filestore paths for artifacts must begin with this prefix.
	ARTIFACT_DEFINITION_PREFIX = api.NewSafeDatastorePath(
		"artifact_definitions").
		SetType(api.PATH_TYPE_DATASTORE_YAML)

	// These store configuration for the server and client
	// monitoring artifacts.
	ServerMonitoringFlowURN = api.NewSafeDatastorePath("config",
		"server_monitoring").SetType(api.PATH_TYPE_DATASTORE_JSON)

	ClientMonitoringFlowURN = api.NewSafeDatastorePath(
		"config", "client_monitoring").SetType(api.PATH_TYPE_DATASTORE_JSON)

	ThirdPartyInventory = api.NewSafeDatastorePath(
		"config", "inventory").SetType(api.PATH_TYPE_DATASTORE_JSON)
)
