package notebook

import (
	"fmt"
	"html"
	"time"

	"google.golang.org/protobuf/proto"
	api_proto "www.velocidex.com/golang/velociraptor/api/proto"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	"www.velocidex.com/golang/velociraptor/reporting"
	"www.velocidex.com/golang/velociraptor/utils"
)

type progressReporter struct {
	config_obj           *config_proto.Config
	notebook_cell        *api_proto.NotebookCell
	notebook_id, version string
	last, start          time.Time

	store NotebookStore
	tmpl  *reporting.GuiTemplateEngine
}

func (self *progressReporter) Report(message string) {
	now := utils.GetTime().Now()
	if now.Before(self.last.Add(4 * time.Second)) {
		return
	}

	self.last = now
	duration := time.Since(self.start).Round(time.Second)

	notebook_cell := proto.Clone(self.notebook_cell).(*api_proto.NotebookCell)
	notebook_cell.Output = fmt.Sprintf(`
<div class="padded"><i class="fa fa-spinner fa-spin fa-fw"></i>
   Calculating...  (%v after %v)
</div>
<div class="panel">
   <velo-csv-viewer base-url="'v1/GetTable'"
                   params='{"notebook_id":"%s","cell_id":"%s","table_id":1,"cell_version": "%s", "message": "%s"}' />
</div>
`,
		html.EscapeString(message),
		html.EscapeString(duration.String()),
		html.EscapeString(self.notebook_id),
		html.EscapeString(self.notebook_cell.CellId),
		html.EscapeString(self.version),
		html.EscapeString(message))
	notebook_cell.Timestamp = now.UnixNano()
	notebook_cell.Duration = int64(duration.Seconds())
	notebook_cell.Messages = self.tmpl.Messages()

	// Cant do anything if we can not set the notebook times
	_ = self.store.SetNotebookCell(self.notebook_id, notebook_cell)
}
