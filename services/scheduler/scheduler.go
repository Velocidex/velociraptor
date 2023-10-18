package scheduler

import (
	"context"
	"errors"
	"fmt"
	"math/rand"
	"sort"
	"sync"
	"time"

	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	"www.velocidex.com/golang/velociraptor/logging"
	"www.velocidex.com/golang/velociraptor/services"
	"www.velocidex.com/golang/velociraptor/utils"
)

type Worker struct {
	id       uint64
	priority int

	output chan services.SchedulerJob
}

type Scheduler struct {
	mu sync.Mutex

	queues map[string][]*Worker
}

func (self *Scheduler) RegisterWorker(
	ctx context.Context, name string, priority int) (
	chan services.SchedulerJob, error) {

	output_chan := make(chan services.SchedulerJob)

	worker := &Worker{
		id:       utils.GetId(),
		priority: priority,
		output:   output_chan,
	}

	go func() {
		defer close(output_chan)

		// Wait here until the context is done and then remove the
		// worker from the queue.
		<-ctx.Done()

		self.mu.Lock()
		defer self.mu.Unlock()

		queues, _ := self.queues[name]
		new_queue := make([]*Worker, 0, len(queues))
		for _, q := range queues {
			if q.id != worker.id {
				new_queue = append(new_queue, q)
			}
		}
		self.queues[name] = new_queue
	}()

	self.mu.Lock()
	queues, _ := self.queues[name]
	queues = append(queues, worker)
	self.queues[name] = queues
	self.mu.Unlock()

	return output_chan, nil
}

func (self *Scheduler) Schedule(ctx context.Context,
	job services.SchedulerJob) (chan services.JobResponse, error) {
	for {
		// The following does not block so we can do it all under lock
		self.mu.Lock()

		// Find a ready worker
		workers, _ := self.queues[job.Queue]
		if len(workers) == 0 {
			self.mu.Unlock()
			return nil, fmt.Errorf("No workers available on queue %v!", job.Queue)
		}

		result_chan := make(chan services.JobResponse)
		job.Done = func(result string, err error) {
			result_chan <- services.JobResponse{
				Job: result,
				Err: err,
			}

			close(result_chan)
		}

		// The following shuffles all workers of the same priority
		// order but higher order is in front.
		rand.Seed(time.Now().UnixNano())
		rand.Shuffle(len(workers), func(i, j int) { workers[i], workers[j] = workers[j], workers[i] })

		// Higher priority (remote) workers first.
		sort.Slice(workers, func(i, j int) bool { return workers[i].priority > workers[j].priority })

		for _, w := range workers {
			select {
			case <-ctx.Done():
				self.mu.Unlock()
				close(result_chan)
				return result_chan, errors.New("Cancelled")
			case w.output <- job:
				// It worked!
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
	return nil
}
