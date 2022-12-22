package reporting

import (
	"context"
	"time"

	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	"www.velocidex.com/golang/velociraptor/datastore"
	"www.velocidex.com/golang/velociraptor/file_store/api"
	"www.velocidex.com/golang/velociraptor/file_store/path_specs"
)

type progressReporter struct {
	ctx            context.Context
	config_obj     *config_proto.Config
	path           api.DSPathSpec
	cancel         func()
	zip_writer     *Container
	container_path api.FSPathSpec

	Type string
}

func (self *progressReporter) writeStats() {
	db, err := datastore.GetDB(self.config_obj)
	if err != nil {
		return
	}
	stats := self.zip_writer.Stats()
	stats.Type = self.Type
	stats.Components = path_specs.AsGenericComponentList(self.container_path)
	_ = db.SetSubject(self.config_obj, self.path, stats)
}

func (self *progressReporter) Close() {
	self.cancel()
}

func NewProgressReporter(
	config_obj *config_proto.Config,
	path api.DSPathSpec,
	container_path api.FSPathSpec,
	zip_writer *Container) *progressReporter {

	// Export happens asynchrounously outside the context of the
	// calling API.
	ctx, cancel := context.WithCancel(context.Background())

	self := &progressReporter{
		ctx:            ctx,
		config_obj:     config_obj,
		path:           path,
		cancel:         cancel,
		zip_writer:     zip_writer,
		container_path: container_path,
		Type:           "zip",
	}

	go func() {
		for {
			select {
			case <-self.ctx.Done():
				self.writeStats()
				return

			case <-time.After(2 * time.Second):
				self.writeStats()
			}
		}
	}()

	// Write the stats immediately to provide feedback that a file is
	// being produces.
	self.writeStats()

	return self
}
