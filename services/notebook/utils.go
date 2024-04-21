package notebook

import (
	"strings"

	api_proto "www.velocidex.com/golang/velociraptor/api/proto"
	"www.velocidex.com/golang/velociraptor/utils"
)

func NewNotebookId() string {
	return "N." + utils.NextId()
}

func NewNotebookCellId() string {
	return "NC." + utils.NextId()
}

func NewNotebookAttachmentId() string {
	return "NA." + utils.NextId()
}

func GetNextVersion(version string) string {
	return utils.NextId()
}

func isGlobalNotebooks(notebook_id string) bool {
	if strings.HasPrefix(notebook_id, "N.F.") || // Flow Notebook
		strings.HasPrefix(notebook_id, "N.H.") || // Hunt Notebook
		strings.HasPrefix(notebook_id, "N.E.") { // Event Notebook
		return false
	}

	return true
}

// Not all values from the real cell are stored in cell summaries.
func SummarizeCell(cell_md *api_proto.NotebookCell) *api_proto.NotebookCell {
	return &api_proto.NotebookCell{
		CellId:            cell_md.CellId,
		Timestamp:         cell_md.Timestamp,
		Type:              cell_md.Type,
		CurrentVersion:    cell_md.CurrentVersion,
		AvailableVersions: cell_md.AvailableVersions,
		Calculating:       cell_md.Calculating,
	}
}
