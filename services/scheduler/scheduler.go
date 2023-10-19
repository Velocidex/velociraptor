package scheduler

import (
	"context"
	"errors"
	"fmt"
	"math/rand"
	"sort"
	"sync"
	"time"

	"github.com/Velocidex/ordereddict"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	"www.velocidex.com/golang/velociraptor/logging"
	"www.velocidex.com/golang/velociraptor/services"
	"www.velocidex.com/golang/velociraptor/services/debug"
	"www.velocidex.com/golang/velociraptor/utils"
	"www.velocidex.com/golang/vfilter"
)

type Worker struct {
	mu sync.Mutex

	id       uint64
	priority int

	// Freestyle name to report in stats
	name string

	org_id string

	// If this is busy we do not assign any requests to it.
	busy bool

	output chan services.SchedulerJob
}

func (self *Worker) SetBusy(b bool) {
	self.mu.Lock()
	defer self.mu.Unlock()

	self.busy = b
}

func (self *Worker) IsBusy() bool {
	self.mu.Lock()
	defer self.mu.Unlock()

	return self.busy
}

type Scheduler struct {
	mu sync.Mutex

	queues map[string][]*Worker
}

func (self *Scheduler) RegisterWorker(
	ctx context.Context, queue, name string, priority int) (
	chan services.SchedulerJob, error) {

	output_chan := make(chan services.SchedulerJob)

	worker := &Worker{
		id:       utils.GetId(),
		priority: priority,
		name:     name,
		output:   output_chan,
	}

	go func() {
		defer close(output_chan)

		// Wait here until the context is done and then remove the
		// worker from the queue.
		<-ctx.Done()

		self.mu.Lock()
		defer self.mu.Unlock()

		queues, _ := self.queues[queue]
		new_queue := make([]*Worker, 0, len(queues))
		for _, q := range queues {
			if q.id != worker.id {
				new_queue = append(new_queue, q)
			}
		}
		self.queues[queue] = new_queue
	}()

	self.mu.Lock()
	queues, _ := self.queues[queue]
	queues = append(queues, worker)
	self.queues[queue] = queues
	self.mu.Unlock()

	return output_chan, nil
}

func (self *Scheduler) WriteProfile(ctx context.Context,
	scope vfilter.Scope, output_chan chan vfilter.Row) {

	self.mu.Lock()
	for queue_name, queues := range self.queues {
		for _, q := range queues {
			output_chan <- ordereddict.NewDict().
				Set("Type", "Worker").
				Set("Name", q.name).
				Set("Queue", queue_name).
				Set("IsBusy", q.IsBusy())
		}
	}
	self.mu.Unlock()
}

func (self *Scheduler) Schedule(ctx context.Context,
	job services.SchedulerJob) (chan services.JobResponse, error) {
	for {
		// The following does not block so we can do it all under lock
		self.mu.Lock()

		var available_workers []*Worker

		// Find a ready worker
		workers, _ := self.queues[job.Queue]
		for _, w := range workers {
			if !w.IsBusy() {
				available_workers = append(available_workers, w)
			}
		}

		if len(available_workers) == 0 {
			self.mu.Unlock()
			return nil, fmt.Errorf("No workers available on queue %v!", job.Queue)
		}

		result_chan := make(chan services.JobResponse)

		// The following shuffles all workers of the same priority
		// order but higher order is in front.
		rand.Seed(time.Now().UnixNano())
		rand.Shuffle(len(available_workers), func(i, j int) {
			available_workers[i], available_workers[j] = available_workers[j], available_workers[i]
		})

		// Higher priority (remote) workers first.
		sort.Slice(available_workers, func(i, j int) bool {
			return available_workers[i].priority > available_workers[j].priority
		})

		for _, w := range available_workers {
			job.Done = func(result string, err error) {
				result_chan <- services.JobResponse{
					Job: result,
					Err: err,
				}

				w.SetBusy(false)
				close(result_chan)
			}

			select {
			case <-ctx.Done():
				self.mu.Unlock()
				close(result_chan)
				return result_chan, errors.New("Cancelled")

			case w.output <- job:
				// It worked! Make this worker busy so it can not be
				// assigned work.
				w.SetBusy(true)
				self.mu.Unlock()
				return result_chan, nil

			default:
				// Cant schedule it immediately, lets try the next
				// worker.
			}
		}
		self.mu.Unlock()

		// No available workers, wait a bit without a lock and try
		// again
		select {
		case <-ctx.Done():
			close(result_chan)
			return result_chan, errors.New("Cancelled")

		case <-time.After(200 * time.Microsecond):
		}
	}
}

func StartSchedulerService(
	ctx context.Context,
	wg *sync.WaitGroup,
	config_obj *config_proto.Config) error {

	if !services.IsMaster(config_obj) {
		logger := logging.GetLogger(config_obj, &logging.FrontendComponent)
		logger.Info("Starting Minion Scheduler Service for %v", services.GetOrgName(config_obj))

		scheduler := &MinionScheduler{
			config_obj: config_obj,
			ctx:        ctx,
		}

		services.RegisterScheduler(scheduler)

		return nil
	}

	logger := logging.GetLogger(config_obj, &logging.FrontendComponent)
	logger.Info("Starting Server Scheduler Service for %v", services.GetOrgName(config_obj))

	scheduler := &Scheduler{
		queues: make(map[string][]*Worker),
	}

	services.RegisterScheduler(scheduler)

	debug.RegisterProfileWriter(scheduler.WriteProfile)

	return nil
}
