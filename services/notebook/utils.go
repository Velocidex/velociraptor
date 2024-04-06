package notebook

import (
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

// Not all values from the real cell are stored in cell summaries.
func SummarizeCell(cell_md *api_proto.NotebookCell) *api_proto.NotebookCell {
	return &api_proto.NotebookCell{
		CellId:            cell_md.CellId,
		Timestamp:         cell_md.Timestamp,
		Type:              cell_md.Type,
		CurrentVersion:    cell_md.CurrentVersion,
		AvailableVersions: cell_md.AvailableVersions,
	}
}
