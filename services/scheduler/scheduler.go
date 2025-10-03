package scheduler

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"sync"
	"time"

	"www.velocidex.com/golang/velociraptor/utils/rand"

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

	// If this is busy we do not assign any requests to it.
	busy bool

	request vfilter.Any

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

func (self *Worker) Request() vfilter.Any {
	self.mu.Lock()
	defer self.mu.Unlock()

	return self.request
}

func (self *Worker) SetRequest(r vfilter.Any) {
	self.mu.Lock()
	defer self.mu.Unlock()

	self.request = r
}

type Scheduler struct {
	mu sync.Mutex

	queues map[string][]*Worker

	config_obj *config_proto.Config
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

	var rows []*ordereddict.Dict

	self.mu.Lock()
	for queue_name, queues := range self.queues {
		for _, q := range queues {
			rows = append(rows, ordereddict.NewDict().
				Set("Type", "Worker").
				Set("Name", q.name).
				Set("Priority", q.priority).
				Set("Queue", queue_name).
				Set("IsBusy", q.IsBusy()).
				Set("Request", q.Request()))
		}
	}
	self.mu.Unlock()

	for _, r := range rows {
		select {
		case <-ctx.Done():
			return
		case output_chan <- r:
		}
	}
}

func (self *Scheduler) AvailableWorkers() int {
	count := 0

	self.mu.Lock()

	for _, workers := range self.queues {
		for _, w := range workers {
			if !w.IsBusy() {
				count++
			}
		}
	}
	self.mu.Unlock()
	return count
}

func (self *Scheduler) Schedule(ctx context.Context,
	job services.SchedulerJob) (chan services.JobResponse, error) {

	var wait_time time.Duration
	if self.config_obj.Defaults != nil {
		config_wait_time := self.config_obj.Defaults.NotebookWaitTimeForWorkerMs
		if config_wait_time > 0 {
			wait_time = time.Millisecond * time.Duration(config_wait_time)
		} else if config_wait_time == 0 {
			wait_time = 10 * time.Second
		} else if config_wait_time < 0 {
			wait_time = 0
		}
	}

	for {
		// The following does not block so we can do it all under lock
		var available_workers []*Worker

		// Retry a few times to get a worker from the queue.
		start := utils.GetTime().Now()
		for {
			self.mu.Lock()

			// Find a ready worker
			workers, _ := self.queues[job.Queue]
			for _, w := range workers {
				if !w.IsBusy() {
					available_workers = append(available_workers, w)
				}
			}

			// Yes we got some workers.
			if len(available_workers) > 0 {
				// Hold the lock on break
				break
			}

			// Do not wait with the lock held
			self.mu.Unlock()

			// Give up after 10 seconds.
			if utils.GetTime().Now().Sub(start) > wait_time {
				return nil, fmt.Errorf(
					"No workers available on queue %v!", job.Queue)
			}

			// Try again soon
			utils.GetTime().Sleep(100 * time.Millisecond)
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
			// The worker can get back to the pool immediately while
			// we wait for our consumer.
			job.Done = func(result string, err error) {
				w.SetBusy(false)
				w.SetRequest(vfilter.Null{})
				defer close(result_chan)

				select {
				case <-ctx.Done():
					return

				case result_chan <- services.JobResponse{
					Job: result,
					Err: err,
				}:
				}
			}

			select {

			// If the caller is cancelled we can return the worker to
			// the pool.
			case <-ctx.Done():
				self.mu.Unlock()
				w.SetBusy(false)
				w.SetRequest(vfilter.Null{})
				close(result_chan)
				return result_chan, errors.New("Cancelled")

			case w.output <- job:
				// It worked! Make this worker busy so it can not be
				// assigned work.
				w.SetRequest(job.Job)
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

		case <-time.After(utils.Jitter(200 * time.Microsecond)):
		}
	}
}

// The scheduler service is only started for the root org. Workers
// will switch orgs as needed.
func StartSchedulerService(
	ctx context.Context,
	wg *sync.WaitGroup,
	config_obj *config_proto.Config) error {

	if !services.IsMaster(config_obj) {
		logger := logging.GetLogger(config_obj, &logging.FrontendComponent)
		logger.Info("Starting Minion Scheduler Service")

		scheduler := &MinionScheduler{
			config_obj: config_obj,
			ctx:        ctx,
		}

		services.RegisterScheduler(scheduler)

		return nil
	}

	logger := logging.GetLogger(config_obj, &logging.FrontendComponent)
	logger.Info("Starting Server Scheduler Service")

	scheduler := &Scheduler{
		queues:     make(map[string][]*Worker),
		config_obj: config_obj,
	}

	services.RegisterScheduler(scheduler)

	debug.RegisterProfileWriter(debug.ProfileWriterInfo{
		Name:          "worker",
		Description:   "Reporting information about current worker tasks.",
		ProfileWriter: scheduler.WriteProfile,
		Categories:    []string{"Global", "Services"},
	})

	return nil
}
