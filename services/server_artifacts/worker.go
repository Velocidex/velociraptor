package server_artifacts

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"github.com/go-errors/errors"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	crypto_proto "www.velocidex.com/golang/velociraptor/crypto/proto"
	flows_proto "www.velocidex.com/golang/velociraptor/flows/proto"
	"www.velocidex.com/golang/velociraptor/logging"
	"www.velocidex.com/golang/velociraptor/services"
	"www.velocidex.com/golang/velociraptor/utils"
)

type ServerArtifactRequest struct {
	// Who is running the query.
	Req               *crypto_proto.FlowRequest
	CollectionContext *flows_proto.ArtifactCollectorContext
}

type ServerArtifactResponse struct {
	SessionId string
}

type ServerArtifactWorker struct {
	name string
}

func (self *ServerArtifactWorker) ProcessUpdateRequest(
	ctx context.Context,
	config_obj *config_proto.Config,
	request *ServerArtifactRequest) (*ServerArtifactResponse, error) {

	if request.Req == nil || request.CollectionContext == nil {
		return nil, utils.InvalidArgError
	}

	collection_context, err := NewCollectionContextManager(
		ctx, config_obj, request.Req, request.CollectionContext)
	if err != nil {
		return nil, err
	}

	go collection_context.StartRefresh()

	defer collection_context.Close(ctx)

	return &ServerArtifactResponse{
		SessionId: request.CollectionContext.SessionId,
	}, collection_context.RunQueries(request.Req.VQLClientActions)
}

func (self *ServerArtifactWorker) Start(
	ctx context.Context,
	config_obj *config_proto.Config,
	name string,
	scheduler services.Scheduler) {

	for {
		err := self.RegisterWorker(ctx, config_obj, name, scheduler)
		if err != nil {
			logger := logging.GetLogger(config_obj, &logging.GUIComponent)
			logger.Info("ServerArtifactWorker: %v", err)
		}

		select {
		case <-ctx.Done():
			logger := logging.GetLogger(config_obj, &logging.GUIComponent)
			logger.Info("ServerArtifactWorker: %v", err)
			return
		case <-time.After(utils.Jitter(10 * time.Second)):
		}
	}
}

// Register a worker with the scheduler and process any tasks.
func (self *ServerArtifactWorker) RegisterWorker(
	ctx context.Context,
	config_obj *config_proto.Config,
	name string,
	scheduler services.Scheduler) error {

	org_manager, err := services.GetOrgManager()
	if err != nil {
		return err
	}

	queue := "ServerArtifact"
	job_chan, err := scheduler.RegisterWorker(
		ctx, queue, name, services.GetWorkerPriority(config_obj))
	if err != nil {
		return err
	}

	logger := logging.GetLogger(config_obj, &logging.FrontendComponent)
	logger.Info("ServerArtifactRunner: <green> Connecting to master scheduler with worker %v</>",
		name)

	for {
		select {
		case <-ctx.Done():
			logger := logging.GetLogger(config_obj, &logging.FrontendComponent)
			logger.Info("ServerArtifactRunner: <red> Disconnecting from master scheduler with worker %v</>", name)
			return nil

		case job, ok := <-job_chan:
			if !ok {
				if job.Done != nil {
					job.Done("", errors.New("Cancellation"))
				}
				return nil
			}

			request := &ServerArtifactRequest{}
			err := json.Unmarshal([]byte(job.Job), request)
			if err != nil ||
				request.CollectionContext == nil ||
				request.Req == nil {
				logger.Error("ServerArtifactRunner: Invalid job request in worker: %v: %v",
					err, job.Job)
				job.Done("", err)
				continue
			}

			// run the query in the correct ORG. We assume ACL
			// checks occur in the GUI so we can only receive
			// valid requests here.
			org_config_obj, err := org_manager.GetOrgConfig(job.OrgId)
			if err != nil {
				if job.Done != nil {
					job.Done("", err)
				}
				continue
			}

			// Call our processor on the correct org.
			resp, err := self.ProcessUpdateRequest(ctx, org_config_obj, request)
			serialized, _ := json.Marshal(resp)
			if resp == nil {
				serialized = nil
			}
			if job.Done != nil {
				job.Done(string(serialized), err)
			}
		}
	}
}

type WorkerPool struct {
	workers []*ServerArtifactWorker
}

func NewWorkerPool(
	ctx context.Context,
	config_obj *config_proto.Config,
	number int64) (*WorkerPool, error) {
	result := &WorkerPool{}

	scheduler, err := services.GetSchedulerService(config_obj)
	if err != nil {
		return nil, err
	}

	for i := int64(0); i < number; i++ {
		worker := &ServerArtifactWorker{}
		name := fmt.Sprintf("local_server_artifact_%d", i)
		if !services.IsMaster(config_obj) {
			name = fmt.Sprintf("%v_server_artifact_%d",
				services.GetNodeName(config_obj.Frontend), i)
		}

		go worker.Start(ctx, config_obj, name, scheduler)
		result.workers = append(result.workers, worker)
	}

	return result, nil
}

func (self *ServerArtifactRunner) Start(
	ctx context.Context,
	config_obj *config_proto.Config,
	wg *sync.WaitGroup) error {

	// Only start this once for all orgs. Otherwise we would have as
	// many workers as orgs. Workers will switch into the correct org
	// for processing
	if !utils.IsRootOrg(config_obj.OrgId) {
		return nil
	}

	// Start a few local workers for notebooks. This also limits
	// concurrency on notebook processing.
	_, err := NewWorkerPool(ctx, config_obj,
		services.GetLocalWorkers(config_obj))
	return err
}
