package downloads

import (
	"context"
	"sync"

	"github.com/Velocidex/ordereddict"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	"www.velocidex.com/golang/velociraptor/file_store/api"
	"www.velocidex.com/golang/velociraptor/reporting"
	"www.velocidex.com/golang/vfilter"
	"www.velocidex.com/golang/vfilter/types"
)

type copyFileJob struct {
	src           api.FSPathSpec
	dest          api.FSPathSpec
	expand_sparse bool
	row           *ordereddict.Dict
	output_chan   chan types.Row
}

type workerJob struct {
	ctx        context.Context
	scope      vfilter.Scope
	config_obj *config_proto.Config
	wg         *sync.WaitGroup
	container  *reporting.Container

	copyFileJob *copyFileJob
}

func (self *workerJob) Run(ctx context.Context) {
	defer self.wg.Done()

	if self.copyFileJob != nil {
		// Copy from the file store at these locations.
		err := copyFile(self.ctx, self.scope, self.config_obj,
			self.container, self.copyFileJob.src,
			self.copyFileJob.dest, self.copyFileJob.expand_sparse)
		if err != nil {
			self.copyFileJob.row.Set("Error", err.Error())

			// Write the error row into the uploads.json file. This
			// will be in addition to the original row.
			self.copyFileJob.output_chan <- self.copyFileJob.row
		}

		return
	}
}

type pool struct {
	wg         sync.WaitGroup
	in_chan    chan *workerJob
	ctx        context.Context
	config_obj *config_proto.Config
	scope      vfilter.Scope
	container  *reporting.Container

	cancel func()
}

func (self *pool) Close() {
	close(self.in_chan)

	self.wg.Wait()

	self.cancel()
}

func (self *pool) copyFile(
	src, dest api.FSPathSpec,
	row *ordereddict.Dict,
	expand_sparse bool,
	output_chan chan types.Row) {
	job := &workerJob{
		copyFileJob: &copyFileJob{
			src:           src,
			dest:          dest,
			expand_sparse: expand_sparse,
			output_chan:   output_chan,
			row:           row,
		},
	}
	self.Run(job)
}

func (self *pool) Run(job *workerJob) {
	job.wg = &self.wg
	job.ctx = self.ctx
	job.scope = self.scope
	job.config_obj = self.config_obj
	job.container = self.container

	// Wait for all chunks to be read.
	self.wg.Add(1)
	select {
	case <-self.ctx.Done():
		return

	case self.in_chan <- job:
		// wg.Done when the job finishes running.
	}
}

func newPool(
	ctx context.Context,
	config_obj *config_proto.Config,
	scope vfilter.Scope,
	container *reporting.Container) *pool {

	subctx, cancel := context.WithCancel(ctx)

	result := &pool{
		in_chan:    make(chan *workerJob),
		ctx:        subctx,
		cancel:     cancel,
		scope:      scope,
		config_obj: config_obj,
		container:  container,
	}

	size := 10
	if config_obj.Defaults != nil &&
		config_obj.Defaults.ExportConcurrency > 0 {
		size = int(config_obj.Defaults.ExportConcurrency)
	}

	// Wait for all workers to quit
	result.wg.Add(size)
	for i := 0; i < size; i++ {
		go func() {
			defer result.wg.Done()

			for {
				select {
				case <-subctx.Done():
					return

				case job, ok := <-result.in_chan:
					if !ok {
						return
					}
					job.Run(subctx)
				}
			}
		}()
	}

	return result
}
