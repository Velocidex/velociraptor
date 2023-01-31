/*

   This service provides a runner to execute artifacts on the server.
   Server artifacts run with high privilege and so only users with the
   COLLECT_SERVER permission can run those.

   Server artifacts are used for administration or information purposes.

*/

package server_artifacts

import (
	"context"
	"sync"
	"time"

	actions_proto "www.velocidex.com/golang/velociraptor/actions/proto"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	crypto_proto "www.velocidex.com/golang/velociraptor/crypto/proto"
	"www.velocidex.com/golang/velociraptor/logging"

	"www.velocidex.com/golang/velociraptor/services"
)

// The Server Artifact Service is responsible for running server side
// VQL artifacts.

// Currently there is only a single server artifact runner, running on
// the master node.

type ServerArtifactsRunner struct {
	config_obj *config_proto.Config
	mu         sync.Mutex

	ctx context.Context
	wg  *sync.WaitGroup

	// Keep track of currently in flight queries so we can cancel
	// them.
	in_flight_collections map[string]CollectionContextManager
}

// Create a bare ServerArtifactsService without the extra management.
func NewServerArtifactRunner(
	ctx context.Context,
	config_obj *config_proto.Config, wg *sync.WaitGroup) *ServerArtifactsRunner {
	return &ServerArtifactsRunner{
		config_obj:            config_obj,
		in_flight_collections: make(map[string]CollectionContextManager),
		ctx:                   ctx,
		wg:                    wg,
	}
}

// Retrieve any tasks waiting in the queues and evaluate them.
func (self *ServerArtifactsRunner) process(
	ctx context.Context,
	config_obj *config_proto.Config,
	wg *sync.WaitGroup) error {

	client_info_manager, err := services.GetClientInfoManager(config_obj)
	if err != nil {
		return err
	}

	messages, err := client_info_manager.GetClientTasks(ctx, "server")
	if err != nil {
		return err
	}

	// We only support two message types - FlowRequest to evaluate a
	// flow and Cancel to cancel it.
	for _, req := range messages {
		session_id := req.SessionId

		if req.Cancel != nil {
			// This collection is now done, cancel it.
			self.Cancel(session_id, req.Cancel.Principal)
			return nil
		}

		if req.FlowRequest != nil {
			sub_ctx, cancel := context.WithCancel(ctx)
			collection_context, err := NewCollectionContextManager(
				sub_ctx, wg, self.config_obj, req)
			if err != nil {
				return err
			}

			wg.Add(1)
			go func() {
				defer wg.Done()
				defer cancel()
				defer collection_context.Save()

				self.ProcessTask(config_obj,
					req.SessionId, collection_context, req.FlowRequest)
			}()
		}
	}

	return nil
}

func (self *ServerArtifactsRunner) Cancel(flow_id, principal string) {
	self.mu.Lock()
	defer self.mu.Unlock()

	context_manager, pres := self.in_flight_collections[flow_id]
	if pres {
		context_manager.Cancel(principal)
		delete(self.in_flight_collections, flow_id)
	}
}

// A single FlowRequest may contain many VQLClientActions, each may
// represents a single source to be run in parallel. The artifact
// compiler will decide how to structure the artifact into multiple
// VQLClientActions (e.g. by considering precondition clauses).
func (self *ServerArtifactsRunner) ProcessTask(
	config_obj *config_proto.Config,
	session_id string,
	collection_context CollectionContextManager,
	req *crypto_proto.FlowRequest) error {

	self.mu.Lock()
	self.in_flight_collections[session_id] = collection_context
	self.mu.Unlock()

	// Wait here for all the queries to exit then remove them from the
	// in_flight_collections map.
	defer func() {
		collection_context.Close()

		self.mu.Lock()
		delete(self.in_flight_collections, session_id)
		self.mu.Unlock()
	}()

	wg := &sync.WaitGroup{}
	for _, task := range req.VQLClientActions {
		// We expect each source to be run in parallel.
		wg.Add(1)
		go func(task *actions_proto.VQLCollectorArgs) {
			collection_context.RunQuery(task)
			wg.Done()
		}(task)
	}

	wg.Wait()

	return nil
}

func NewServerArtifactService(
	ctx context.Context,
	wg *sync.WaitGroup,
	config_obj *config_proto.Config) error {

	self := NewServerArtifactRunner(ctx, config_obj, wg)

	logger := logging.GetLogger(
		config_obj, &logging.FrontendComponent)
	logger.Info("<green>Starting</> Server Artifact Runner Service")

	notifier, err := services.GetNotifier(config_obj)
	if err != nil {
		return err
	}

	wg.Add(1)
	go func() {
		defer wg.Done()

		logger := logging.GetLogger(config_obj, &logging.FrontendComponent)

		// Listen for notifications from the server.
		notification, cancel := notifier.ListenForNotification("server")
		defer cancel()

		err := self.process(ctx, config_obj, wg)
		if err != nil {
			logger.Error("ServerArtifactsRunner: %v", err)
			return
		}

		for {
			select {
			// Check the queues anyway every minute in case we miss the
			// notification.
			case <-time.After(time.Duration(60) * time.Second):
				err = self.process(ctx, config_obj, wg)
				if err != nil {
					logger.Error("ServerArtifactsRunner: %v", err)
					continue
				}

			case <-ctx.Done():
				return

			case quit := <-notification:
				if quit {
					logger.Info("ServerArtifactsRunner: quit.")
					return
				}
				err := self.process(ctx, config_obj, wg)
				if err != nil {
					logger.Error("ServerArtifactsRunner: %v", err)
					continue
				}

				// Listen again.
				cancel()
				notification, cancel = notifier.ListenForNotification("server")
			}
		}
	}()

	return nil
}
