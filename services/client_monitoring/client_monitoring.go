// Velociraptor clients stream monitoring events to the server. This
// is controlled by the ClientEventTable below and can be updated by
// the GUI at any time.
//
// This service maintains access to the global event table.
package client_monitoring

import (
	"context"
	"math/rand"
	"sync"

	actions_proto "www.velocidex.com/golang/velociraptor/actions/proto"
	"www.velocidex.com/golang/velociraptor/artifacts"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	"www.velocidex.com/golang/velociraptor/constants"
	crypto_proto "www.velocidex.com/golang/velociraptor/crypto/proto"
	"www.velocidex.com/golang/velociraptor/datastore"
	flows_proto "www.velocidex.com/golang/velociraptor/flows/proto"
	"www.velocidex.com/golang/velociraptor/logging"
	"www.velocidex.com/golang/velociraptor/services"
	"www.velocidex.com/golang/velociraptor/utils"
)

type ClientEventTable struct {
	mu sync.Mutex

	// The state table maintains all the artifacts we collect from
	// clients. There are two main parts:

	// 1. Artifacts is an ArtifactCollectorArgs specifying those
	//    artifacts we collect from ALL clients.
	// 2. LabelEvents is a list of ArtifactCollectorArgs keyed by
	//    labels. If a client matches the label it will collect
	//    those artifactd as well.

	// Each of these ArtifactCollectorArgs cache a list of
	// VQLCollectorArgs - the actual messages sent to clients
	// containing compiled artifacts. Therefore assigning the
	// client a VQLEventTable message is very cheap. All we need
	// to do is:

	// 1. Copy all the CompiledCollectorArgs from the global
	//    Artifacts member,
	// 2. Iterate over all the LabelEvents, check the client has
	//    the label and if it does, copy the CompiledCollectorArgs
	//    from this specific LabelEvent.

	// Since checking labels is a memory operation and we just
	// copy pointers around it is very fast and can be done on
	// every client efficiently.

	// We therefore rely on pre-compiling artifacts during the
	// SetClientMonitoringState() call, then keep the compiled
	// protobufs in memory.
	state *flows_proto.ClientEventTable

	ctx        context.Context
	config_obj *config_proto.Config
	repository *artifacts.Repository

	clock utils.Clock
}

// Checks to see if we need to update the client event table.
func (self *ClientEventTable) CheckClientEventsVersion(
	client_id string, client_version uint64) bool {
	self.mu.Lock()
	defer self.mu.Unlock()

	labeler := services.GetLabeler()
	if client_version < self.state.Version {
		return true
	}

	// If the client's labels have changed after their table
	// timestamp, then they will need to update as well.
	if client_version < labeler.LastLabelTimestamp(client_id) {
		return true
	}

	return false
}

func (self *ClientEventTable) GetClientMonitoringState() *flows_proto.ClientEventTable {
	self.mu.Lock()
	defer self.mu.Unlock()

	return self.state
}

func (self *ClientEventTable) SetClientMonitoringState(
	state *flows_proto.ClientEventTable) error {
	self.mu.Lock()
	defer self.mu.Unlock()

	return self.setClientMonitoringState(state)
}

func (self *ClientEventTable) compileArtifactCollectorArgs(
	artifact *flows_proto.ArtifactCollectorArgs) (
	[]*actions_proto.VQLCollectorArgs, error) {
	// Make a local copy.

	result := []*actions_proto.VQLCollectorArgs{}
	launcher := services.GetLauncher()

	// Compile each artifact separately into its own VQLCollectorArgs so they can be run in parallel.
	for _, name := range artifact.Artifacts {
		temp := *artifact
		temp.Artifacts = []string{name}
		compiled, err := launcher.CompileCollectorArgs(
			self.ctx, self.config_obj,
			self.config_obj.Client.PinnedServerName, // Principal
			self.repository, &temp)
		if err != nil {
			return nil, err
		}

		result = append(result, compiled)
	}
	return result, nil
}

func (self *ClientEventTable) compileState(state *flows_proto.ClientEventTable) (err error) {
	// Compile all the artifacts now for faster dispensing.
	compiled, err := self.compileArtifactCollectorArgs(state.Artifacts)
	if err != nil {
		return err
	}
	state.Artifacts.CompiledCollectorArgs = compiled

	// Now compile the label specific events
	for _, table := range state.LabelEvents {
		compiled, err := self.compileArtifactCollectorArgs(table.Artifacts)
		if err != nil {
			return err
		}
		table.Artifacts.CompiledCollectorArgs = compiled
	}

	return nil
}

func (self *ClientEventTable) setClientMonitoringState(
	state *flows_proto.ClientEventTable) error {

	self.state = state
	state.Version = uint64(self.clock.Now().UnixNano())

	// Store the new table in the data store.
	db, err := datastore.GetDB(self.config_obj)
	if err != nil {
		return err
	}

	err = self.compileState(self.state)
	if err != nil {
		return err
	}

	err = db.SetSubject(self.config_obj, constants.ClientMonitoringFlowURN,
		self.state)
	if err != nil {
		return err
	}

	// Notify all the client monitoring tables that we got
	// updated. This should cause all frontends to refresh.
	return services.NotifyListener(self.config_obj, constants.ClientMonitoringFlowURN)
}

func (self *ClientEventTable) GetClientUpdateEventTableMessage(
	client_id string) *crypto_proto.GrrMessage {
	self.mu.Lock()
	defer self.mu.Unlock()

	result := &actions_proto.VQLEventTable{
		Version: uint64(self.clock.Now().UnixNano()),
	}

	for _, compiled := range self.state.Artifacts.CompiledCollectorArgs {
		result.Event = append(result.Event, compiled)
	}

	// Now apply any event queries that belong to this client based on labels.
	labeler := services.GetLabeler()
	for _, table := range self.state.LabelEvents {
		if labeler.IsLabelSet(client_id, table.Label) {
			for _, compiled := range table.Artifacts.CompiledCollectorArgs {
				result.Event = append(result.Event, compiled)
			}
		}
	}

	// Add a bit of randomness to the max wait to spread out
	// client's updates so they do not syncronize load on the
	// server.
	for _, event := range result.Event {
		event.MaxWait += uint64(rand.Intn(20))
	}

	return &crypto_proto.GrrMessage{
		UpdateEventTable: result,
		SessionId:        constants.MONITORING_WELL_KNOWN_FLOW,
	}
}

func (self *ClientEventTable) LoadFromFile() error {
	self.mu.Lock()
	defer self.mu.Unlock()

	logger := logging.GetLogger(self.config_obj, &logging.FrontendComponent)
	db, err := datastore.GetDB(self.config_obj)
	if err != nil {
		return err
	}

	self.state = &flows_proto.ClientEventTable{}
	err = db.GetSubject(self.config_obj,
		constants.ClientMonitoringFlowURN, self.state)
	if err != nil || self.state.Version == 0 {
		// No client monitoring rules found, install some
		// defaults.
		self.state.Artifacts = &flows_proto.ArtifactCollectorArgs{
			Artifacts: self.config_obj.Frontend.DefaultClientMonitoringArtifacts,
		}
		logger.Info("Creating default Client Monitoring Service")

		err = self.compileState(self.state)
		if err != nil {
			return err
		}

		return self.setClientMonitoringState(self.state)
	}

	return self.compileState(self.state)
}

// Runs at frontend start to initialize the client monitoring table.
func StartClientMonitoringService(
	ctx context.Context,
	wg *sync.WaitGroup,
	config_obj *config_proto.Config) error {

	repository, err := artifacts.GetGlobalRepository(config_obj)
	if err != nil {
		return err
	}

	event_table := &ClientEventTable{
		config_obj: config_obj,
		ctx:        ctx,
		repository: repository,
		clock:      &utils.RealClock{},
	}

	services.RegisterClientEventManager(event_table)

	logger := logging.GetLogger(config_obj, &logging.FrontendComponent)

	notification, cancel := services.ListenForNotification(
		constants.ClientMonitoringFlowURN)
	defer cancel()

	wg.Add(1)
	go func() {
		defer wg.Done()

		for {
			select {
			case <-ctx.Done():
				return

			case <-notification:
				err := event_table.LoadFromFile()
				if err != nil {
					logger.Error("StartClientMonitoringService: ", err)
					return
				}
			}

			cancel()
			notification, cancel = services.ListenForNotification(
				constants.ClientMonitoringFlowURN)
		}
	}()

	logger.Info("<green>Starting</> Client Monitoring Service")
	return event_table.LoadFromFile()
}
