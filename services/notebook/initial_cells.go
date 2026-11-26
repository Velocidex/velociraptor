package notebook

import (
	"fmt"
	"html"

	"github.com/Velocidex/ordereddict"
	api_proto "www.velocidex.com/golang/velociraptor/api/proto"
	"www.velocidex.com/golang/velociraptor/paths"
	"www.velocidex.com/golang/velociraptor/utils"
)

func DefaultEventCellOutput(
	notebook_metadata *api_proto.NotebookMetadata,
	cell_idx int,
	artifact_name, source_name string,
) string {
	full_source_name := paths.FullArtifactName(artifact_name, source_name)
	if cell_idx > 4 {
		return fmt.Sprintf("<h3>%s</h3><br>Recalculate to View", full_source_name)
	}

	return fmt.Sprintf("<h3>%s</h3><br>Recalculate to View", full_source_name)
}

func DefaultFlowCellOutput(
	notebook_metadata *api_proto.NotebookMetadata,
	cell_idx int,
	artifact_name, source_name string,
) string {
	full_source_name := paths.FullArtifactName(artifact_name, source_name)
	if cell_idx > 4 {
		return fmt.Sprintf("<h3>%s</h3><br>Recalculate to View", full_source_name)
	}

	// If this is a client notebook, just return the bare table - it
	// will be fully populated with the current data.
	flow_id, client_id, ok := utils.ClientNotebookId(notebook_metadata.NotebookId)
	if ok {
		options := ordereddict.NewDict().
			Set("client_id", client_id).
			Set("flow_id", flow_id).
			Set("artifact", full_source_name).
			Set("rows", 10).
			Set("Version", utils.GetTime().Now().UnixNano())

		return fmt.Sprintf(`
<h1>%v</h1>
 <div class="panel"><velo-csv-viewer base-url="'v1/GetTable'" `+
			`params='%s' /></div>`,
			html.EscapeString(full_source_name),
			utils.QueryEscape(options.String()))
	}

	return fmt.Sprintf("<h3>%s</h3><br>Recalculate to View", full_source_name)
}
