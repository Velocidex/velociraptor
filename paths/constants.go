package paths

import "www.velocidex.com/golang/velociraptor/file_store/api"

var (
	// The GUI uses this as the client index.
	CLIENT_INDEX_URN = api.NewSafeDatastorePath("client_index")

	// An index of all the hunts and clients.
	HUNT_INDEX     = api.NewSafeDatastorePath("hunt_index")
	NOTEBOOK_INDEX = api.NewSafeDatastorePath("notebook_index")
	USER_URN       = api.NewSafeDatastorePath("users")

	// Timelines
	TIMELINE_URN = api.NewSafeDatastorePath("timelines")

	// Filestore paths for artifacts must begin with this prefix.
	ARTIFACT_DEFINITION_PREFIX = api.NewSafeDatastorePath(
		"artifact_definitions")

	// These store configuration for the server and client
	// monitoring artifacts.
	ServerMonitoringFlowURN = api.NewSafeDatastorePath(
		"config", "server_monitoring.json")

	ClientMonitoringFlowURN = api.NewSafeDatastorePath(
		"config", "client_monitoring.json")

	ThirdPartyInventory = api.NewSafeDatastorePath(
		"config", "inventory.json")
)
