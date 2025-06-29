/*

   This service provides a runner to execute artifacts on the server.
   Server artifacts run with high privilege and so only users with the
   COLLECT_SERVER permission can run those, unless the artifact is
   marked as BASIC by the artifact metadata.

   Server artifacts are used for administration or information purposes.

*/

package server_artifacts

import (
	"context"
	"sync"

	actions_proto "www.velocidex.com/golang/velociraptor/actions/proto"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	crypto_proto "www.velocidex.com/golang/velociraptor/crypto/proto"
	flows_proto "www.velocidex.com/golang/velociraptor/flows/proto"
	"www.velocidex.com/golang/velociraptor/logging"
	"www.velocidex.com/golang/velociraptor/services"
)

// The Server Artifact Service is responsible for running server side
// VQL artifacts.

// Currently there is only a single server artifact runner (for each
// org), running on the master node.

type ServerArtifactRunner struct {
	mu sync.Mutex

	ctx context.Context
	wg  *sync.WaitGroup

	// Keep track of currently in flight queries so we can cancel
	// them.
	in_flight_collections map[string]CollectionContextManager
}

// Create a bare ServerArtifactsService without the extra management.
func NewServerArtifactRunner(
	ctx context.Context,
	config_obj *config_proto.Config, wg *sync.WaitGroup) *ServerArtifactRunner {
	return &ServerArtifactRunner{
		in_flight_collections: make(map[string]CollectionContextManager),
		ctx:                   ctx,
		wg:                    wg,
	}
}

// Start a new collection in the current process.
func (self *ServerArtifactRunner) LaunchServerArtifact(
	config_obj *config_proto.Config,
	session_id string,
	req *crypto_proto.FlowRequest,
	collection_context *flows_proto.ArtifactCollectorContext) error {

	collection_context_manager, err := NewCollectionContextManager(
		self.ctx, self.wg, config_obj, req, collection_context)
	if err != nil {
		return err
	}

	// Install the manager now so it is available for cancellation
	self.mu.Lock()
	self.in_flight_collections[session_id] = collection_context_manager
	self.mu.Unlock()

	sub_ctx, cancel := context.WithCancel(self.ctx)

	collection_context_manager.StartRefresh(self.wg)

	self.wg.Add(1)
	go func() {
		defer self.wg.Done()
		defer cancel()
		defer collection_context_manager.Close(self.ctx)

		err := self.ProcessTask(sub_ctx, config_obj,
			session_id, collection_context_manager, req)
		if err != nil {
			logger := logging.GetLogger(config_obj, &logging.FrontendComponent)
			logger.Error("<red>ServerArtifactRunner ProcessTask</> %v", err)
		}
	}()

	return nil
}

func (self *ServerArtifactRunner) Cancel(
	ctx context.Context, flow_id, principal string) {
	self.mu.Lock()
	defer self.mu.Unlock()

	context_manager, pres := self.in_flight_collections[flow_id]
	if pres {
		context_manager.Cancel(ctx, principal)
		delete(self.in_flight_collections, flow_id)
	}
}

// A single FlowRequest may contain many VQLClientActions, each may
// represents a single source to be run in parallel. The artifact
// compiler will decide how to structure the artifact into multiple
// VQLClientActions (e.g. by considering precondition clauses).
func (self *ServerArtifactRunner) ProcessTask(
	ctx context.Context, config_obj *config_proto.Config,
	session_id string,
	collection_context CollectionContextManager,
	req *crypto_proto.FlowRequest) error {

	var (
		err error
		mu  sync.Mutex
	)

	defer collection_context.Close(ctx)

	// Wait here for all the queries to exit then remove them from the
	// in_flight_collections map.
	defer func() {
		collection_context.Close(ctx)

		self.mu.Lock()
		delete(self.in_flight_collections, session_id)
		self.mu.Unlock()
	}()

	wg := &sync.WaitGroup{}
	for _, task := range req.VQLClientActions {
		// We expect each source to be run in parallel.
		wg.Add(1)
		go func(task *actions_proto.VQLCollectorArgs) {
			defer wg.Done()

			err1 := collection_context.RunQuery(task)
			if err1 != nil {
				mu.Lock()
				if err == nil {
					err = err1
				}
				mu.Unlock()
			}
		}(task)
	}

	wg.Wait()

	mu.Lock()
	defer mu.Unlock()
	return err
}

func NewServerArtifactService(
	ctx context.Context,
	wg *sync.WaitGroup,
	config_obj *config_proto.Config) (services.ServerArtifactRunner, error) {

	self := NewServerArtifactRunner(ctx, config_obj, wg)

	logger := logging.GetLogger(
		config_obj, &logging.FrontendComponent)
	logger.Info("<green>Starting</> Server Artifact Runner Service for %v",
		services.GetOrgName(config_obj))

	return self, nil
}
