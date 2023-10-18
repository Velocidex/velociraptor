package scheduler

import (
	"context"
	"errors"
	"sync"

	api_proto "www.velocidex.com/golang/velociraptor/api/proto"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	"www.velocidex.com/golang/velociraptor/logging"
	"www.velocidex.com/golang/velociraptor/services"
)

type MinionScheduler struct {
	mu         sync.Mutex
	config_obj *config_proto.Config
	ctx        context.Context
}

func NewMinionScheduler(
	config_obj *config_proto.Config,
	ctx context.Context) *MinionScheduler {
	return &MinionScheduler{
		config_obj: config_obj,
		ctx:        ctx,
	}
}

// Connect to the server and bind the local worker with the server
func (self *MinionScheduler) RegisterWorker(
	ctx context.Context, name string, priority int) (
	chan services.SchedulerJob, error) {

	logger := logging.GetLogger(self.config_obj, &logging.FrontendComponent)

	frontend_manager, err := services.GetFrontendManager(self.config_obj)
	if err != nil {
		return nil, err
	}

	// Get a new API handle each time in case it became invalid.
	api_client, closer, err := frontend_manager.GetMasterAPIClient(
		self.ctx)
	if err != nil {
		return nil, err
	}
	defer closer()

	stream, err := api_client.Scheduler(self.ctx)
	if err != nil {
		return nil, err
	}

	// Register the worker on the server
	err = stream.Send(&api_proto.ScheduleRequest{
		Queue: name,
		Type:  "register",
	})
	if err != nil {
		logger.Error("MinionScheduler: Unable to register worker for %v: %v",
			name, err)
		return nil, err
	}

	logger.Info("MinionScheduler: Registered worker for <green>%v</>", name)

	output_chan := make(chan services.SchedulerJob)
	go func() {
		defer close(output_chan)

		for {
			req, err := stream.Recv()
			if err != nil {
				return
			}

			select {
			case <-ctx.Done():
				return
			case output_chan <- services.SchedulerJob{
				Queue: req.Queue,
				Job:   req.Job,
				Done: func(result string, err error) {
					err_str := ""
					if err != nil {
						err_str = err.Error()
					}

					stream.Send(
						&api_proto.ScheduleRequest{
							Queue:    req.Queue,
							Type:     "response",
							Id:       req.Id,
							Response: result,
							Error:    err_str,
						})
				},
			}:
			}
		}
	}()

	return output_chan, nil
}

func (self *MinionScheduler) Schedule(ctx context.Context,
	job services.SchedulerJob) (chan services.JobResponse, error) {
	return nil, errors.New("MinionScheduler does not support direct scheduling.")
}

// Make a test connection to make sure that we actually can connect to
// the server.
func (self *MinionScheduler) Start() error {
	frontend_manager, err := services.GetFrontendManager(self.config_obj)
	if err != nil {
		return err
	}

	// Get a new API handle each time in case it became invalid.
	api_client, closer, err := frontend_manager.GetMasterAPIClient(
		self.ctx)
	if err != nil {
		return err
	}
	defer closer()

	stream, err := api_client.Scheduler(self.ctx)
	if err != nil {
		return err
	}

	// Register the worker on the server
	err = stream.Send(&api_proto.ScheduleRequest{
		Queue: "Test",
		Type:  "register",
	})
	return err
}
