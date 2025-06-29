package scheduler

import (
	"context"
	"errors"

	api_proto "www.velocidex.com/golang/velociraptor/api/proto"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	"www.velocidex.com/golang/velociraptor/logging"
	"www.velocidex.com/golang/velociraptor/services"
	"www.velocidex.com/golang/velociraptor/utils"
)

type MinionScheduler struct {
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
	ctx context.Context, queue, name string, priority int) (
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
	defer func() {
		_ = closer()
	}()

	stream, err := api_client.Scheduler(self.ctx)
	if err != nil {
		return nil, err
	}

	// Register the worker on the server
	err = stream.Send(&api_proto.ScheduleRequest{
		Queue:    queue,
		Priority: int64(priority),
		Type:     "register",
	})
	if err != nil {
		logger.Error("MinionScheduler: Unable to register worker for %v: %v",
			queue, err)
		return nil, err
	}

	logger.Info("MinionScheduler: Registered worker for <green>%v</>", queue)

	output_chan := make(chan services.SchedulerJob)
	go func() {
		defer close(output_chan)

		for {
			req, err := stream.Recv()
			if err != nil {
				return
			}
			start := utils.GetTime().Now()
			select {
			case <-ctx.Done():
				return

			case output_chan <- services.SchedulerJob{
				Queue: req.Queue,
				Job:   req.Job,
				OrgId: req.OrgId,
				Done: func(result string, err error) {
					err_str := ""
					if err != nil {
						err_str = err.Error()
					}
					logger.Debug("MinionScheduler: Completed job for queue <green>%v</> in %v on %v",
						queue, utils.GetTime().Now().Sub(start), utils.NormalizedOrgId(req.OrgId))
					_ = stream.Send(
						&api_proto.ScheduleRequest{
							Queue:    req.Queue,
							Type:     "response",
							Id:       req.Id,
							Response: result,
							Error:    err_str,
						})
				},
			}:
				logger.Debug("MinionScheduler: Received job for queue <green>%v</> in %v",
					queue, utils.NormalizedOrgId(req.OrgId))
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
	defer func() {
		_ = closer()
	}()

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
