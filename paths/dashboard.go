package paths

import "www.velocidex.com/golang/velociraptor/file_store/api"

// Dashboards may live in a global place or a client specific place
func NewDashboardPathManager(dashboard_type, artifact,
	client_id string) *NotebookCellPathManager {

	if dashboard_type == "ARTIFACT_DESCRIPTION" {
		return &NotebookCellPathManager{
			notebook_id: "Dashboards",
			cell_id:     artifact,
			root: NOTEBOOK_ROOT.
				SetType(api.PATH_TYPE_DATASTORE_JSON),
		}
	}

	// The Dashboard name is based on the root of the artifact name
	artifact, _ = SplitFullSourceName(artifact)

	if client_id == "" {
		return &NotebookCellPathManager{
			notebook_id: "Dashboards",
			cell_id:     artifact,
			client_id:   client_id,
			root: NOTEBOOK_ROOT.
				SetType(api.PATH_TYPE_DATASTORE_JSON),
		}
	}

	return &NotebookCellPathManager{
		client_id:   client_id,
		cell_id:     artifact,
		notebook_id: "Dashboards",
		root: CLIENTS_ROOT.AddUnsafeChild(client_id).
			SetType(api.PATH_TYPE_DATASTORE_JSON),
	}
}
