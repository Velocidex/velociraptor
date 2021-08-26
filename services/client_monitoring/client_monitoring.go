// Velociraptor clients stream monitoring events to the server. This
// is controlled by the ClientEventTable below and can be updated by
// the GUI at any time.
//
// This service maintains access to the global event table.

// NOTE: The client's event table will be updated when the client's
// table's version if one the following is changed:
// 1. The global event table state was modified (eg. the user updated the GUI).

// 2. Any label was updated for that client which may have caused the
// client to be added into the label group.

// 3. The artifact was deleted or updated.

package client_monitoring

import (
	"context"
	"errors"
	"math/rand"
	"sync"

	"github.com/Velocidex/ordereddict"
	"github.com/google/uuid"
	"google.golang.org/protobuf/proto"
	actions_proto "www.velocidex.com/golang/velociraptor/actions/proto"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	"www.velocidex.com/golang/velociraptor/constants"
	crypto_proto "www.velocidex.com/golang/velociraptor/crypto/proto"
	"www.velocidex.com/golang/velociraptor/datastore"
	flows_proto "www.velocidex.com/golang/velociraptor/flows/proto"
	"www.velocidex.com/golang/velociraptor/logging"
	"www.velocidex.com/golang/velociraptor/paths"
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
	//    those artifacts as well.

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

	Clock utils.Clock

	id string
}

// Checks to see if we need to update the client event table. Each
// client's table version is the timestamp when it received the event
// table update. Clients need to renew their table if:
// 1. Their version is behind the the global table version, or
// 2. Their version is behind the latest label update.
//
// When the table is refreshed its version is set to the current
// timestamp which implied after both of these conditions.
func (self *ClientEventTable) CheckClientEventsVersion(
	config_obj *config_proto.Config,
	client_id string, client_version uint64) bool {

	self.mu.Lock()
	version := self.state.Version
	self.mu.Unlock()

	if client_version < version {
		return true
	}

	// Now check the label group
	labeler := services.GetLabeler()
	if labeler == nil {
		return false
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

	launcher, err := services.GetLauncher()
	if err != nil {
		return nil, err
	}

	manager, err := services.GetRepositoryManager()
	if err != nil {
		return nil, err
	}

	repository, err := manager.GetGlobalRepository(config_obj)
	if err != nil {
		return nil, err
	}

	return launcher.CompileCollectorArgs(
		ctx, config_obj, vql_subsystem.NullACLManager{},
		repository, services.CompilerOptions{
			ObfuscateNames:         true,
			IgnoreMissingArtifacts: true,
		}, artifact)
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
			logger := logging.GetLogger(config_obj, &logging.FrontendComponent)
			logger.Error("Unable to start client monitoring service: Error "+
				"compiling artifacts %v: %v", table.Artifacts, err)
			logger.Error("Please correct client_monitoring config file at " +
				"<datastore>/config/client_monitoring.json.db")
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
	state.Version = uint64(self.Clock.Now().UnixNano())

	// Store the new table in the data store.
	db, err := datastore.GetDB(config_obj)
	if err != nil {
		return err
	}

	err = self.compileState(ctx, config_obj, self.state)
	if err != nil {
		return err
	}

	err = db.SetSubject(config_obj, paths.ClientMonitoringFlowURN,
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
	client_id string) *crypto_proto.VeloMessage {
	self.mu.Lock()
	state := self.state
	self.mu.Unlock()

	result := &actions_proto.VQLEventTable{
		Version: uint64(self.Clock.Now().UnixNano()),
	}

	if state.Artifacts == nil {
		state.Artifacts = &flows_proto.ArtifactCollectorArgs{}
	}

	for _, event := range state.Artifacts.CompiledCollectorArgs {
		result.Event = append(result.Event, proto.Clone(event).(*actions_proto.VQLCollectorArgs))
	}

	// Now apply any event queries that belong to this client based on labels.
	labeler := services.GetLabeler()
	for _, table := range state.LabelEvents {
		if labeler.IsLabelSet(config_obj, client_id, table.Label) {
			for _, event := range table.Artifacts.CompiledCollectorArgs {
				result.Event = append(result.Event,
					proto.Clone(event).(*actions_proto.VQLCollectorArgs))
			}
		}
	}

	// Add a bit of randomness to the max wait to spread out
	// client's updates so they do not syncronize load on the
	// server.
	for _, event := range result.Event {
		// Ensure responses do not come back too quickly
		// because this increases the load on the server. We
		// need the client to queue at least 60 seconds worth
		// of data before reconnecting.
		if event.MaxWait < 60 {
			event.MaxWait = 120
		}

		event.MaxWait += uint64(rand.Intn(20))

		// Event queries never time out
		event.Timeout = 99999999
	}

	return &crypto_proto.VeloMessage{
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

	logger := logging.GetLogger(config_obj, &logging.FrontendComponent)
	logger.Info("Updating Client Event Table because %v was updated", modified_name)

	setter, _ := event.GetString("setter")

	// Determine if the modified artifact affects us.
	is_relevant := func() bool {
		// Ignore events that we sent ourselves.
		if setter == self.id {
			return false
		}

		// We could try to figure out if the artifact actually changed
		// anything but this is hard to know - not only do we need to
		// look at the artifact in the event table but all
		// dependencies as well. So for now we just recompile the
		// event table when any artifact is changed. We dont expect
		// this to be too frequent.
		return true
	}

	if is_relevant() {
		err := self.load_from_file(ctx, config_obj)
		if err != nil {
			logger := logging.GetLogger(
				config_obj, &logging.FrontendComponent)
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

	return self.load_from_file(ctx, config_obj)
}

func (self *ClientEventTable) load_from_file(
	ctx context.Context, config_obj *config_proto.Config) error {
	logger := logging.GetLogger(config_obj, &logging.FrontendComponent)
	logger.Info("Reloading client monitoring tables from datastore\n")
	db, err := datastore.GetDB(config_obj)
	if err != nil {
		return err
	}

	self.state = &flows_proto.ClientEventTable{}
	err = db.GetSubject(config_obj,
		paths.ClientMonitoringFlowURN, self.state)
	if err != nil || self.state.Version == 0 {
		// No client monitoring rules found, install some
		// defaults.
		self.state.Artifacts = &flows_proto.ArtifactCollectorArgs{
			Artifacts: config_obj.Frontend.DefaultClientMonitoringArtifacts,
		}
		self.state.LabelEvents = append(self.state.LabelEvents,
			&flows_proto.LabelEvents{
				Label: "Quarantine",
				Artifacts: &flows_proto.ArtifactCollectorArgs{
					Artifacts: []string{
						"Windows.Remediation.QuarantineMonitor",
					},
				},
			})
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

	event_table := &ClientEventTable{
		Clock: &utils.RealClock{},
		id:    uuid.New().String(),
	}
	services.RegisterClientEventManager(event_table)

	logger := logging.GetLogger(config_obj, &logging.FrontendComponent)
	logger.Info("<green>Starting</> Client Monitoring Service")
	journal, err := services.GetJournal()
	if err != nil {
		return err
	}

	events, cancel := journal.Watch(ctx, "Server.Internal.ArtifactModification")

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

	return event_table.LoadFromFile(ctx, config_obj)
}
