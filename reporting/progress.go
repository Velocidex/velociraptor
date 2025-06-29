package reporting

import (
	"context"
	"sync"
	"time"

	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	"www.velocidex.com/golang/velociraptor/file_store/api"
	"www.velocidex.com/golang/velociraptor/file_store/path_specs"
	"www.velocidex.com/golang/velociraptor/services"
)

type progressReporter struct {
	// Upstream context allows us to watch for cancellation of the
	// container writer.
	ctx context.Context

	// Signal stop for periodic stats updates.
	mu             sync.Mutex
	error          string
	config_obj     *config_proto.Config
	zip_writer     *Container
	container_path api.FSPathSpec
	opts           services.ContainerOptions

	Type string
}

func (self *progressReporter) writeStats() {
	self.mu.Lock()
	defer self.mu.Unlock()

	export_manager, err := services.GetExportManager(self.config_obj)
	if err != nil {
		return
	}

	stats := self.zip_writer.Stats()
	stats.Type = self.Type
	stats.Components = path_specs.AsGenericComponentList(self.container_path)
	stats.Error = self.error

	_ = export_manager.SetContainerStats(self.ctx, self.config_obj, stats, self.opts)
}

// If we call Close() before the timeout then the collection worked
// fine - even when the context is cancelled.
func (self *progressReporter) Close() {
	// Write the stats one final time.
	self.writeStats()

	self.mu.Lock()
	defer self.mu.Unlock()

	if self.error == "" {
		self.error = "Complete"
	}
}

func NewProgressReporter(
	ctx context.Context,
	config_obj *config_proto.Config,
	container_path api.FSPathSpec,
	opts services.ContainerOptions,
	zip_writer *Container) *progressReporter {

	self := &progressReporter{
		ctx:            ctx,
		config_obj:     config_obj,
		zip_writer:     zip_writer,
		container_path: container_path,
		opts:           opts,
		Type:           "zip",
	}

	go func() {
		for {
			select {
			case <-ctx.Done():

				// Update the error stats if the context has timed
				// out.
				self.mu.Lock()
				if self.error == "" {
					self.error = "Timeout"
				}
				self.mu.Unlock()

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
