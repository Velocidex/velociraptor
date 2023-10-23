package services

import (
	"context"
	"errors"

	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
)

var (
	global_scheduler Scheduler
)

func RegisterScheduler(scheduler Scheduler) {
	mu.Lock()
	defer mu.Unlock()

	global_scheduler = scheduler
}

func GetSchedulerService(config_obj *config_proto.Config) (Scheduler, error) {
	mu.Lock()
	defer mu.Unlock()

	if global_scheduler == nil {
		return nil, errors.New("Scheduler not initialized")
	}

	return global_scheduler, nil
}

type SchedulerJob struct {
	// The queue that is being scheduled.
	Queue string

	// A JSON encoded free form that is interpreted depending on the queue used.
	Job string

	OrgId string

	// When the worker receives the job, on completion the worker must
	// call the Done function with an error status.
	Done func(result string, err error)
}

type JobResponse struct {
	Job string
	Err error
}

// Manages scheduling to distributed workers.
type Scheduler interface {
	// Call this to register a worker. The caller will receive a
	// channel over which jobs will be distributed. When the context
	// is Done, the channel will be closed.
	//
	// When the worker completes the task they need to call job.Done()
	RegisterWorker(ctx context.Context,
		name, queue string, priority int) (chan SchedulerJob, error)

	// Called by code that wants to schedule the job. The job will be
	// scheduled to one of the available workers.
	Schedule(ctx context.Context, job SchedulerJob) (chan JobResponse, error)
}
