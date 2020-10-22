// Velociraptor clients stream monitoring events to the server. This
// is controlled by the ClientEventTable below and can be updated by
// the GUI at any time.
//
// This service maintains access to the global event table.
package client_monitoring

import (
	"context"
	"errors"
	"math/rand"
	"sync"

	"github.com/Velocidex/ordereddict"
	"github.com/google/uuid"
	actions_proto "www.velocidex.com/golang/velociraptor/actions/proto"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	"www.velocidex.com/golang/velociraptor/constants"
	crypto_proto "www.velocidex.com/golang/velociraptor/crypto/proto"
	"www.velocidex.com/golang/velociraptor/datastore"
	flows_proto "www.velocidex.com/golang/velociraptor/flows/proto"
	"www.velocidex.com/golang/velociraptor/logging"
	"www.velocidex.com/golang/velociraptor/services"
	"www.velocidex.com/golang/velociraptor/utils"
	vql_subsystem "www.velocidex.com/golang/velociraptor/vql"
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

	repository services.Repository

	clock utils.Clock

	id string
}

// Checks to see if we need to update the client event table.
func (self *ClientEventTable) CheckClientEventsVersion(
	config_obj *config_proto.Config,
	client_id string, client_version uint64) bool {
	self.mu.Lock()
	defer self.mu.Unlock()

	labeler := services.GetLabeler()
	if labeler == nil {
		return false
	}
	if client_version < self.state.Version {
		return true
	}

	// If the client's labels have changed after their table
	// timestamp, then they will need to update as well.
	if client_version < labeler.LastLabelTimestamp(config_obj, client_id) {
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
	ctx context.Context,
	config_obj *config_proto.Config,
	state *flows_proto.ClientEventTable) error {
	self.mu.Lock()
	defer self.mu.Unlock()

	return self.setClientMonitoringState(ctx, config_obj, state)
}

func (self *ClientEventTable) compileArtifactCollectorArgs(
	ctx context.Context,
	config_obj *config_proto.Config,
	artifact *flows_proto.ArtifactCollectorArgs) (
	[]*actions_proto.VQLCollectorArgs, error) {

	result := []*actions_proto.VQLCollectorArgs{}
	launcher, err := services.GetLauncher()
	if err != nil {
		return nil, err
	}
	// Compile each artifact separately into its own
	// VQLCollectorArgs so they can be run in parallel.
	for _, name := range artifact.Artifacts {
		// Make a local copy.
		temp := *artifact
		temp.Artifacts = []string{name}
		compiled, err := launcher.CompileCollectorArgs(
			ctx, config_obj, vql_subsystem.NullACLManager{},
			self.repository,
			true, /* should_obfuscate */
			&temp)
		if err != nil {
			return nil, err
		}

		result = append(result, compiled)
	}
	return result, nil
}

func (self *ClientEventTable) compileState(
	ctx context.Context,
	config_obj *config_proto.Config,
	state *flows_proto.ClientEventTable) (err error) {
	if state.Artifacts == nil {
		state.Artifacts = &flows_proto.ArtifactCollectorArgs{}
	}

	// Compile all the artifacts now for faster dispensing.
	compiled, err := self.compileArtifactCollectorArgs(
		ctx, config_obj, state.Artifacts)
	if err != nil {
		return err
	}
	state.Artifacts.CompiledCollectorArgs = compiled

	// Now compile the label specific events
	for _, table := range state.LabelEvents {
		compiled, err := self.compileArtifactCollectorArgs(
			ctx, config_obj, table.Artifacts)
		if err != nil {
			return err
		}
		table.Artifacts.CompiledCollectorArgs = compiled
	}

	return nil
}

func (self *ClientEventTable) setClientMonitoringState(
	ctx context.Context,
	config_obj *config_proto.Config,
	state *flows_proto.ClientEventTable) error {

	if state.Artifacts == nil {
		state.Artifacts = &flows_proto.ArtifactCollectorArgs{}
	}

	self.state = state
	state.Version = uint64(self.clock.Now().UnixNano())

	// Store the new table in the data store.
	db, err := datastore.GetDB(config_obj)
	if err != nil {
		return err
	}

	err = self.compileState(ctx, config_obj, self.state)
	if err != nil {
		return err
	}

	err = db.SetSubject(config_obj, constants.ClientMonitoringFlowURN,
		self.state)
	if err != nil {
		return err
	}

	// Notify all the client monitoring tables that we got
	// updated. This should cause all frontends to refresh.
	journal, err := services.GetJournal()
	if err != nil {
		return err
	}

	return journal.PushRowsToArtifact(config_obj,
		[]*ordereddict.Dict{
			ordereddict.NewDict().
				Set("setter", self.id).
				Set("artifact", "ClientEventTable").
				Set("op", "set"),
		}, "Server.Internal.ArtifactModification", "", "")
}

func (self *ClientEventTable) GetClientUpdateEventTableMessage(
	config_obj *config_proto.Config,
	client_id string) *crypto_proto.GrrMessage {
	self.mu.Lock()
	defer self.mu.Unlock()

	result := &actions_proto.VQLEventTable{
		Version: uint64(self.clock.Now().UnixNano()),
	}

	if self.state.Artifacts == nil {
		self.state.Artifacts = &flows_proto.ArtifactCollectorArgs{}
	}

	result.Event = append(result.Event,
		self.state.Artifacts.CompiledCollectorArgs...)

	// Now apply any event queries that belong to this client based on labels.
	labeler := services.GetLabeler()
	for _, table := range self.state.LabelEvents {
		if labeler.IsLabelSet(config_obj, client_id, table.Label) {
			result.Event = append(
				result.Event, table.Artifacts.CompiledCollectorArgs...)
		}
	}

	// Add a bit of randomness to the max wait to spread out
	// client's updates so they do not syncronize load on the
	// server.
	for _, event := range result.Event {
		event.MaxWait += uint64(rand.Intn(20))

		// Event queries never time out
		event.Timeout = 99999999
	}

	return &crypto_proto.GrrMessage{
		UpdateEventTable: result,
		SessionId:        constants.MONITORING_WELL_KNOWN_FLOW,
	}
}

func (self *ClientEventTable) ProcessArtifactModificationEvent(
	ctx context.Context,
	config_obj *config_proto.Config, event *ordereddict.Dict) {
	self.mu.Lock()
	defer self.mu.Unlock()

	modified_name, pres := event.GetString("artifact")
	if !pres || modified_name == "" {
		return
	}

	setter, _ := event.GetString("setter")

	// Determine if the modified artifact affects us.
	is_relevant := func() bool {
		// Ignore events that we sent.
		if setter == self.id {
			return false
		}

		if modified_name == "ClientEventTable" {
			return true
		}

		if utils.InString(self.state.Artifacts.Artifacts, modified_name) {
			return true
		}

		for _, label_event := range self.state.LabelEvents {
			if label_event.Artifacts != nil {
				if utils.InString(label_event.Artifacts.Artifacts, modified_name) {
					return true
				}
			}
		}
		return false
	}

	if is_relevant() {
		// Recompile artifacts and update the version.
		self.state.Version = uint64(self.clock.Now().UnixNano())

		clear_caches(self.state)
		err := self.compileState(ctx, config_obj, self.state)
		if err != nil {
			logger := logging.GetLogger(config_obj, &logging.FrontendComponent)
			logger.Error("compileState: %v", err)
		}
	}
}

// Clear all the pre-compiled VQLCollectorArgs - next time we sent the
// artifact it will be rebuilt.
func clear_caches(state *flows_proto.ClientEventTable) {
	if state.Artifacts == nil {
		state.Artifacts = &flows_proto.ArtifactCollectorArgs{}
	}

	state.Artifacts.CompiledCollectorArgs = nil
	for _, event := range state.LabelEvents {
		event.Artifacts.CompiledCollectorArgs = nil
	}
}

func (self *ClientEventTable) LoadFromFile(
	ctx context.Context, config_obj *config_proto.Config) error {
	self.mu.Lock()
	defer self.mu.Unlock()

	if config_obj.Frontend == nil {
		return errors.New("Frontend not configured")
	}

	logger := logging.GetLogger(config_obj, &logging.FrontendComponent)
	db, err := datastore.GetDB(config_obj)
	if err != nil {
		return err
	}

	self.state = &flows_proto.ClientEventTable{}
	err = db.GetSubject(config_obj,
		constants.ClientMonitoringFlowURN, self.state)
	if err != nil || self.state.Version == 0 {
		// No client monitoring rules found, install some
		// defaults.
		self.state.Artifacts = &flows_proto.ArtifactCollectorArgs{
			Artifacts: config_obj.Frontend.DefaultClientMonitoringArtifacts,
		}
		logger.Info("Creating default Client Monitoring Service")

		err = self.compileState(ctx, config_obj, self.state)
		if err != nil {
			return err
		}

		return self.setClientMonitoringState(ctx, config_obj, self.state)
	}

	clear_caches(self.state)
	return self.compileState(ctx, config_obj, self.state)
}

// Runs at frontend start to initialize the client monitoring table.
func StartClientMonitoringService(
	ctx context.Context,
	wg *sync.WaitGroup,
	config_obj *config_proto.Config) error {

	manager, err := services.GetRepositoryManager()
	if err != nil {
		return err
	}

	repository, err := manager.GetGlobalRepository(config_obj)
	if err != nil {
		return err
	}

	event_table := &ClientEventTable{
		repository: repository,
		clock:      &utils.RealClock{},
		id:         uuid.New().String(),
	}

	logger := logging.GetLogger(config_obj, &logging.FrontendComponent)
	logger.Info("<green>Starting</> Client Monitoring Service")
	journal, err := services.GetJournal()
	if err != nil {
		return err
	}

	events, cancel := journal.Watch("Server.Internal.ArtifactModification")

	wg.Add(1)
	go func() {
		defer wg.Done()
		defer cancel()
		defer services.RegisterClientEventManager(nil)

		for {
			select {
			case <-ctx.Done():
				return

			case event, ok := <-events:
				if !ok {
					return
				}
				event_table.ProcessArtifactModificationEvent(
					ctx, config_obj, event)
			}
		}
	}()

	services.RegisterClientEventManager(event_table)

	return event_table.LoadFromFile(ctx, config_obj)
}
