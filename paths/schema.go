package paths

import (
	"fmt"
	"strings"

	"github.com/Velocidex/ordereddict"
	"www.velocidex.com/golang/velociraptor/file_store/api"
	"www.velocidex.com/golang/velociraptor/utils"
)

type ObjectType string

const (
	DATASTORE_OBJECT ObjectType = "DatastoreObject"
	RESULT_SET       ObjectType = "ResultSet"
	TIMED_RESULT_SET ObjectType = "TimedResultSet"
	BULK_DATA        ObjectType = "BulkData"
	ZIP_FILE         ObjectType = "ZipFile"
	YAML_FILE        ObjectType = "YAML_FILE"

	// Markers to signify specific patterns
	CLIENT_ID        = "CLIENT_ID"
	FLOW_ID          = "FLOW_ID"
	HUNT_ID          = "HUNT_ID"
	NOTEBOOK_ID      = "NOTEBOOK_ID"
	NOTEBOOK_CELL_ID = "NOTEBOOK_CELL_ID"
	ARTIFACT_NAME    = "ARTIFACT_NAME"
	SOURCE_NAME      = "SOURCE_NAME"

	ZIP_FILE_PATTERN  = "*.zip"
	HTML_FILE_PATTERN = "*.html"
	WILD_CARD         = ""
)

// Map the file store paths into descriptors which describe what data
// is supposed to be there.
type PathDescriptor struct {
	Components []string
	Recursive  bool
	Type       string
	PathType   ObjectType

	Decorator func(api.FSPathSpec) *ordereddict.Dict
}

var (
	Schema = []PathDescriptor{
		{
			Components: []string{"public"},
			Type:       "Public",
			Recursive:  true,
			PathType:   BULK_DATA,
		},
		{
			Components: []string{"backups"},
			Type:       "Backups",
			Recursive:  true,
			PathType:   ZIP_FILE,
		},
		{
			Components: []string{"temp"},
			Type:       "Temp",
			Recursive:  true,
			PathType:   BULK_DATA,
		},

		// Flows
		{
			Components: []string{"clients", CLIENT_ID, "collections", FLOW_ID},
			Type:       "Flow Context",
			PathType:   DATASTORE_OBJECT,
			Decorator:  extractClientIdFlowId,
		},
		{
			Components: []string{"clients", CLIENT_ID, "collections", FLOW_ID, "logs"},
			Type:       "Flow Logs",
			PathType:   RESULT_SET,
			Decorator:  extractClientIdFlowId,
		},
		{
			// The flow start request.
			Components: []string{"clients", CLIENT_ID, "collections", FLOW_ID, "requests"},
			Type:       "Flow Requests",
			PathType:   DATASTORE_OBJECT,
			Decorator:  extractClientIdFlowId,
		},
		{
			// The compiled flow actually sent to the client.
			Components: []string{"clients", CLIENT_ID, "collections", FLOW_ID, "task"},
			Type:       "Flow Task",
			PathType:   DATASTORE_OBJECT,
			Decorator:  extractClientIdFlowId,
		},
		{
			Components: []string{"clients", CLIENT_ID, "collections", FLOW_ID, "stats"},
			Type:       "Flow Stats",
			PathType:   DATASTORE_OBJECT,
			Decorator:  extractClientIdFlowId,
		},

		{
			// A list of files uploaded into the flow
			Components: []string{"clients", CLIENT_ID, "collections", FLOW_ID, "uploads"},
			Type:       "Flow Upload Metadata",
			PathType:   RESULT_SET,
			Decorator:  extractClientIdFlowId,
		},
		{
			Components: []string{"clients", CLIENT_ID, "collections", FLOW_ID, "upload_transactions"},
			Type:       "Flow Upload Transactions",
			PathType:   RESULT_SET,
			Decorator:  extractClientIdFlowId,
		},
		{
			Components: []string{"clients", CLIENT_ID, "collections", FLOW_ID, "uploads"},
			Type:       "Flow Uploads",
			Recursive:  true,
			PathType:   BULK_DATA,
			Decorator:  extractClientIdFlowId,
		},

		// Flow data exports
		{
			Components: []string{"downloads", CLIENT_ID, FLOW_ID, ZIP_FILE_PATTERN},
			Type:       "Export Download",
			PathType:   ZIP_FILE,
			Decorator:  extractClientIdFlowId,
		},
		{
			Components: []string{"downloads", CLIENT_ID, FLOW_ID, WILD_CARD},
			Type:       "Export Download Metadata",
			PathType:   DATASTORE_OBJECT,
			Decorator:  extractClientIdFlowId,
		},

		// Flow notebooks
		{
			Components: []string{"clients", CLIENT_ID, "collections", FLOW_ID, "notebook",
				NOTEBOOK_ID},
			Type:      "Client Notebook",
			PathType:  DATASTORE_OBJECT,
			Decorator: extractClientIdFlowId,
		},
		{
			Components: []string{"clients", CLIENT_ID, "collections", FLOW_ID, "notebook",
				NOTEBOOK_ID, "artifact"},
			Type:      "Client Notebook Artifact",
			PathType:  DATASTORE_OBJECT,
			Decorator: extractClientIdFlowId,
		},
		{
			Components: []string{"clients", CLIENT_ID, "collections",
				FLOW_ID, "notebook", NOTEBOOK_ID, NOTEBOOK_CELL_ID},
			Type:      "Client Notebook Cell",
			Recursive: true,
			PathType:  BULK_DATA,
			Decorator: extractClientIdFlowId,
		},
		{
			Components: []string{"clients", CLIENT_ID, "collections", FLOW_ID, "notebook",
				NOTEBOOK_ID, "attach"},
			Type:      "Client Notebook Attachments",
			Recursive: true,
			PathType:  BULK_DATA,
			Decorator: extractClientIdFlowId,
		},
		{
			Components: []string{"clients", CLIENT_ID, "collections", FLOW_ID, "notebook",
				NOTEBOOK_ID, "uploads"},
			Type:      "Client Notebook Attachments",
			Recursive: true,
			PathType:  BULK_DATA,
			Decorator: extractClientIdFlowId,
		},

		// Notebook exports
		{
			Components: []string{"downloads", "notebooks", NOTEBOOK_ID, ZIP_FILE_PATTERN},
			Type:       "Client Notebook Export (ZIP)",
			PathType:   ZIP_FILE,
		},
		{
			Components: []string{"downloads", "notebooks", NOTEBOOK_ID, HTML_FILE_PATTERN},
			Type:       "Client Notebook Export (HTML)",
			PathType:   ZIP_FILE,
		},
		{
			Components: []string{"downloads", "notebooks", NOTEBOOK_ID, WILD_CARD},
			Type:       "Client Notebook Export Metadata",
			PathType:   DATASTORE_OBJECT,
		},

		// Global notebooks
		{
			Components: []string{"notebooks", NOTEBOOK_ID},
			Type:       "Notebook",
			PathType:   DATASTORE_OBJECT,
		},
		{
			Components: []string{"notebooks", NOTEBOOK_ID, "artifact"},
			Type:       "Notebook Artifact",
			PathType:   DATASTORE_OBJECT,
		},
		{
			Components: []string{"notebooks", NOTEBOOK_ID, NOTEBOOK_CELL_ID},
			Type:       "Notebook Cell",
			Recursive:  true,
			PathType:   BULK_DATA,
		},
		{
			Components: []string{"notebooks", NOTEBOOK_ID, "attach"},
			Type:       "Notebook Attachments",
			Recursive:  true,
			PathType:   BULK_DATA,
		},
		{
			Components: []string{"notebook", NOTEBOOK_ID, "uploads"},
			Type:       "Notebook Attachments",
			Recursive:  true,
			PathType:   BULK_DATA,
		},

		// Client information
		{
			Components: []string{"clients", CLIENT_ID},
			Type:       "Client Metadata",
			PathType:   DATASTORE_OBJECT,
			Decorator:  extractClientId,
		},
		{
			Components: []string{"clients", CLIENT_ID, "key"},
			Type:       "Client Public Key",
			PathType:   DATASTORE_OBJECT,
			Decorator:  extractClientId,
		},
		{
			Components: []string{"clients", CLIENT_ID, "flow_index"},
			Type:       "Client Flow Index",
			PathType:   RESULT_SET,
			Decorator:  extractClientId,
		},
		{
			Components: []string{"clients", CLIENT_ID, "tasks", WILD_CARD},
			Type:       "Client Scheduled Tasks",
			PathType:   DATASTORE_OBJECT,
			Decorator:  extractClientId,
		},
		{
			// For result sets with a source name
			Components: []string{"clients", CLIENT_ID, "artifacts", ARTIFACT_NAME,
				FLOW_ID, SOURCE_NAME},
			Type:      "Client collected artifacts",
			PathType:  RESULT_SET,
			Decorator: extractClientIdFlowIdArtifactName,
		},
		{
			// For result sets with an empty source name
			Components: []string{"clients", CLIENT_ID, "artifacts", ARTIFACT_NAME, FLOW_ID},
			Type:       "Client collected artifacts",
			PathType:   RESULT_SET,
			Decorator:  extractClientIdFlowIdArtifactName,
		},
		{
			Components: []string{"clients", CLIENT_ID, "monitoring",
				ARTIFACT_NAME, WILD_CARD},
			Type:      "Client collected monitoring artifacts",
			PathType:  TIMED_RESULT_SET,
			Decorator: extractMonitoringClientIdArtifactName,
		},
		{
			Components: []string{"clients", CLIENT_ID, "monitoring",
				ARTIFACT_NAME, SOURCE_NAME, WILD_CARD},
			Type:      "Client collected monitoring artifacts",
			PathType:  TIMED_RESULT_SET,
			Decorator: extractMonitoringClientIdArtifactName,
		},
		{
			Components: []string{"clients", CLIENT_ID, "monitoring_logs",
				ARTIFACT_NAME, WILD_CARD},
			Type:      "Client monitoring artifact logs",
			PathType:  TIMED_RESULT_SET,
			Decorator: extractMonitoringClientIdArtifactName,
		},
		{
			Components: []string{"clients", CLIENT_ID, "monitoring_logs",
				ARTIFACT_NAME, SOURCE_NAME, WILD_CARD},
			Type:      "Client monitoring artifact logs",
			PathType:  TIMED_RESULT_SET,
			Decorator: extractMonitoringClientIdArtifactName,
		},
		{
			Components: []string{"client_info", "snapshot"},
			Type:       "Client metadata snapshot",
			PathType:   RESULT_SET,
		},
		{
			Components: []string{"artifact_definitions"},
			Recursive:  true,
			Type:       "Artifact Definitions (Custom)",
			PathType:   YAML_FILE,
		},

		// Hunts
		{
			Components: []string{"hunts", HUNT_ID},
			Type:       "Hunt Metadata",
			PathType:   DATASTORE_OBJECT,
		},
		{
			Components: []string{"hunts", HUNT_ID},
			Type:       "Hunt Participation",
			PathType:   RESULT_SET,
		},
		{
			Components: []string{"hunts", HUNT_ID, "enriched"},
			Type:       "Hunt Client Details",
			PathType:   RESULT_SET,
		},
		{
			Components: []string{"hunts", HUNT_ID, "request"},
			Type:       "Hunt Request",
			PathType:   DATASTORE_OBJECT,
		},

		// Hunt notebooks
		{
			Components: []string{"hunts", HUNT_ID, "notebook",
				NOTEBOOK_ID},
			Type:     "Client Notebook",
			PathType: DATASTORE_OBJECT,
		},
		{
			Components: []string{"hunts", HUNT_ID, "notebook",
				NOTEBOOK_ID, "artifact"},
			Type:     "Client Notebook Artifact",
			PathType: DATASTORE_OBJECT,
		},
		{
			Components: []string{"hunts", HUNT_ID, "notebook",
				NOTEBOOK_ID, NOTEBOOK_CELL_ID},
			Type:      "Client Notebook Cell",
			Recursive: true,
			PathType:  BULK_DATA,
		},
		{
			Components: []string{"hunts", HUNT_ID, "notebook", NOTEBOOK_ID, "attach"},
			Type:       "Client Notebook Attachments",
			Recursive:  true,
			PathType:   BULK_DATA,
		},
		{
			Components: []string{"hunts", HUNT_ID, "notebook", NOTEBOOK_ID, "uploads"},
			Type:       "Client Notebook Attachments",
			Recursive:  true,
			PathType:   BULK_DATA,
		},

		// Server monitoring
		{
			Components: []string{"server_artifacts", ARTIFACT_NAME, WILD_CARD},
			Type:       "Server Monitoring",
			PathType:   TIMED_RESULT_SET,
			Decorator:  extractMonitoringServerArtifactName,
		},
		{
			Components: []string{"server_artifacts", ARTIFACT_NAME, SOURCE_NAME, WILD_CARD},
			Type:       "Server Monitoring",
			PathType:   TIMED_RESULT_SET,
			Decorator:  extractMonitoringServerArtifactName,
		},
		{
			Components: []string{"server_artifact_logs", ARTIFACT_NAME, WILD_CARD},
			Type:       "Server Monitoring Logs",
			PathType:   TIMED_RESULT_SET,
			Decorator:  extractMonitoringServerArtifactName,
		},
		{
			Components: []string{"server_artifact_logs", ARTIFACT_NAME, SOURCE_NAME, WILD_CARD},
			Type:       "Server Monitoring Logs",
			PathType:   TIMED_RESULT_SET,
			Decorator:  extractMonitoringServerArtifactName,
		},
	}
)

func DescribePath(path api.FSPathSpec) PathDescriptor {
	switch path.Type() {
	case api.PATH_TYPE_FILESTORE_CHUNK_INDEX,
		api.PATH_TYPE_FILESTORE_JSON_INDEX,
		api.PATH_TYPE_FILESTORE_SPARSE_IDX,
		api.PATH_TYPE_FILESTORE_JSON_TIME_INDEX:
		return PathDescriptor{
			PathType: ObjectType(path.Type().String()),
		}
	}

	for _, schema := range Schema {
		if matchComponents(path.Components(), schema) {
			return schema
		}
	}
	return PathDescriptor{
		Type:     "Unknown",
		PathType: BULK_DATA,
	}
}

func matchComponents(path []string, schema PathDescriptor) bool {
	// For recursive schema, we compare the prefix.
	if schema.Recursive {
		if len(path) < len(schema.Components) {
			return false
		}
		path = path[:len(schema.Components)]

		// Otherwise it has to be an exact match.
	} else if len(path) != len(schema.Components) {
		return false
	}

	// Here path and schema.Components have the same length - compare
	// component by component.
	for idx, path_component := range path {
		schema_component := schema.Components[idx]

		switch schema_component {
		case CLIENT_ID:
			if !strings.HasPrefix(path_component, "C.") &&
				path_component != "server" {
				return false
			}
		case FLOW_ID:
			if !strings.HasPrefix(path_component, "F.") {
				return false
			}
		case HUNT_ID:
			if !strings.HasPrefix(path_component, "H.") {
				return false
			}
		case NOTEBOOK_ID:
			if !strings.HasPrefix(path_component, "N.") {
				return false
			}
		case ARTIFACT_NAME, SOURCE_NAME:
			fmt.Printf("Validating %v\n", path_component)

			if utils.ValidateArtifactName(path_component) != nil {
				return false
			}
		case ZIP_FILE_PATTERN:
			if !strings.HasSuffix(path_component, ".zip") {
				return false
			}
		case HTML_FILE_PATTERN:
			if !strings.HasSuffix(path_component, ".html") {
				return false
			}
		default:
			if schema_component != WILD_CARD &&
				path_component != schema_component {
				return false
			}
		}
	}

	return true
}
