package notebook

import (
	"regexp"

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

var (
	summary_regex = []*regexp.Regexp{
		// The first heading is a good summary.
		regexp.MustCompile("<h[1-4]>(.+?)</h"),
	}
)

// Not all values from the real cell are stored in cell summaries.
func SummarizeCell(cell_md *api_proto.NotebookCell) *api_proto.NotebookCell {
	var summary string

	// Derive the summary of the cell by its output.
	for _, re := range summary_regex {
		matches := re.FindStringSubmatch(cell_md.Output)
		if len(matches) > 1 {
			summary = matches[1]
			break
		}
	}

	return &api_proto.NotebookCell{
		CellId:            cell_md.CellId,
		Summary:           summary,
		Timestamp:         cell_md.Timestamp,
		Type:              cell_md.Type,
		CurrentVersion:    cell_md.CurrentVersion,
		AvailableVersions: cell_md.AvailableVersions,
		Calculating:       cell_md.Calculating,
	}
}
