package notebook

import (
	"fmt"
	"time"

	"google.golang.org/protobuf/proto"
	api_proto "www.velocidex.com/golang/velociraptor/api/proto"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
)

type progressReporter struct {
	config_obj            *config_proto.Config
	notebook_cell         *api_proto.NotebookCell
	notebook_id, table_id string
	last, start           time.Time

	store NotebookStore
}

func (self *progressReporter) Report(message string) {
	now := time.Now()
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
   <grr-csv-viewer base-url="'v1/GetTable'"
                   params='{"notebook_id":"%s","cell_id":"%s","table_id":1,"message": "%s"}' />
</div>
`,
		message, duration,
		self.notebook_id, self.notebook_cell.CellId, message)
	notebook_cell.Timestamp = now.Unix()
	notebook_cell.Duration = int64(duration.Seconds())

	// Cant do anything if we can not set the notebook times
	_ = self.store.SetNotebookCell(self.notebook_id, notebook_cell)
}
