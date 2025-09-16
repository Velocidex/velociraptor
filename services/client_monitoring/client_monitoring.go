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
	"sync"

	"www.velocidex.com/golang/velociraptor/utils/rand"

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
	"www.velocidex.com/golang/velociraptor/vql/acl_managers"
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

	// There is a separate manager for each org.
	config_obj *config_proto.Config
	id         string
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
	ctx context.Context,
	config_obj *config_proto.Config,
	client_id string, client_version uint64) bool {

	self.mu.Lock()
	version := self.state.Version
	self.mu.Unlock()

	if client_version < version {
		return true
	}

	// Now check the label group
	labeler := services.GetLabeler(config_obj)
	if labeler == nil {
		return false
	}

	// If the client's labels have changed after their table
	// timestamp, then they will need to update as well.
	if client_version < labeler.LastLabelTimestamp(ctx, config_obj, client_id) {
		return true
	}

	return false
}

func (self *ClientEventTable) GetClientMonitoringState() *flows_proto.ClientEventTable {
	self.mu.Lock()
	defer self.mu.Unlock()

	return proto.Clone(self.state).(*flows_proto.ClientEventTable)
}

func (self *ClientEventTable) SetClientMonitoringState(
	ctx context.Context,
	config_obj *config_proto.Config,
	principal string,
	state *flows_proto.ClientEventTable) error {
	self.mu.Lock()
	defer self.mu.Unlock()

	return self.setClientMonitoringState(ctx, config_obj, principal, state)
}

func compileArtifactCollectorArgs(
	ctx context.Context,
	config_obj *config_proto.Config,
	artifact *flows_proto.ArtifactCollectorArgs) (
	[]*actions_proto.VQLCollectorArgs, error) {

	launcher, err := services.GetLauncher(config_obj)
	if err != nil {
		return nil, err
	}

	manager, err := services.GetRepositoryManager(config_obj)
	if err != nil {
		return nil, err
	}

	repository, err := manager.GetGlobalRepository(config_obj)
	if err != nil {
		return nil, err
	}

	var log_delay uint64
	if config_obj.Frontend != nil &&
		config_obj.Frontend.Resources != nil &&
		config_obj.Frontend.Resources.DefaultMonitoringLogBatchTime > 0 {
		log_delay = config_obj.Frontend.Resources.DefaultMonitoringLogBatchTime
	}

	return launcher.CompileCollectorArgs(
		ctx, config_obj, acl_managers.NullACLManager{},
		repository, services.CompilerOptions{
			ObfuscateNames:         true,
			IgnoreMissingArtifacts: true,
			LogBatchTime:           log_delay,
		}, artifact)
}

func compileState(ctx context.Context, config_obj *config_proto.Config,
	state *flows_proto.ClientEventTable) (err error) {
	if state.Artifacts == nil {
		state.Artifacts = &flows_proto.ArtifactCollectorArgs{}
	}

	// Compile all the artifacts now for faster dispensing.
	compiled, err := compileArtifactCollectorArgs(
		ctx, config_obj, state.Artifacts)
	if err != nil {
		return err
	}
	state.Artifacts.CompiledCollectorArgs = compiled

	// Now compile the label specific events
	for _, table := range state.LabelEvents {
		compiled, err := compileArtifactCollectorArgs(
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
	principal string,
	state *flows_proto.ClientEventTable) error {

	if state.Artifacts == nil {
		state.Artifacts = &flows_proto.ArtifactCollectorArgs{}
	}

	self.state = proto.Clone(state).(*flows_proto.ClientEventTable)
	self.state.Version = uint64(utils.GetTime().Now().UnixNano())

	// Store the new table in the data store.
	db, err := datastore.GetDB(config_obj)
	if err != nil {
		return err
	}

	err = compileState(ctx, config_obj, self.state)
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
	journal, err := services.GetJournal(config_obj)
	if err != nil {
		return err
	}

	if principal != "" {
		err := services.LogAudit(ctx,
			config_obj, principal, "SetClientMonitoringState",
			ordereddict.NewDict().
				Set("user", principal).
				Set("state", self.state))
		if err != nil {
			return err
		}
	}

	err = journal.PushRowsToArtifact(ctx, config_obj,
		[]*ordereddict.Dict{
			ordereddict.NewDict().
				Set("setter", self.id).
				Set("principal", principal).
				Set("artifact", "ClientEventTable").
				Set("op", "set"),
		}, "Server.Internal.ArtifactModification", "", "")
	if err != nil {
		return err
	}

	// This does not happen usually - on when running in GUI mode
	// because we need low latency there.
	if config_obj.Defaults.EventChangeNotifyAllClients {
		notifier, err := services.GetNotifier(config_obj)
		if err != nil {
			return err
		}

		for _, c := range notifier.ListClients() {
			notifier.NotifyDirectListener(c)
		}
	}

	return nil
}

func (self *ClientEventTable) GetClientSpec(
	ctx context.Context,
	config_obj *config_proto.Config,
	client_id string) (res []*flows_proto.ArtifactSpec) {

	self.mu.Lock()
	state := self.state
	self.mu.Unlock()

	// First get the all labels
	if state.Artifacts != nil {
		res = append(res, state.Artifacts.Specs...)
	}

	labeler := services.GetLabeler(config_obj)
	// Now label specific specs.
	for _, table := range state.LabelEvents {
		if labeler.IsLabelSet(ctx, config_obj, client_id, table.Label) &&
			table.Artifacts != nil {
			res = append(res, table.Artifacts.Specs...)
		}
	}
	return res
}

func (self *ClientEventTable) GetClientUpdateEventTableMessage(
	ctx context.Context,
	config_obj *config_proto.Config,
	client_id string) *crypto_proto.VeloMessage {
	self.mu.Lock()
	state := self.state
	self.mu.Unlock()

	result := &actions_proto.VQLEventTable{
		Version: uint64(utils.GetTime().Now().UnixNano()),
	}

	if state.Artifacts == nil {
		state.Artifacts = &flows_proto.ArtifactCollectorArgs{}
	}

	for _, event := range state.Artifacts.CompiledCollectorArgs {
		result.Event = append(result.Event, proto.Clone(event).(*actions_proto.VQLCollectorArgs))
	}

	// Now apply any event queries that belong to this client based on labels.
	labeler := services.GetLabeler(config_obj)
	for _, table := range state.LabelEvents {
		if labeler.IsLabelSet(ctx, config_obj, client_id, table.Label) {
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
		if event.MaxWait == 0 {
			event.MaxWait = config_obj.Defaults.EventMaxWait
		}

		if event.MaxWait == 0 {
			event.MaxWait = 120
		}

		jitter := config_obj.Defaults.EventMaxWaitJitter
		if jitter == 0 {
			jitter = 20
		}
		event.MaxWait += uint64(rand.Intn(int(jitter)))
	}

	return &crypto_proto.VeloMessage{
		UpdateEventTable: result,
		SessionId:        constants.MONITORING_WELL_KNOWN_FLOW,
	}
}

func (self *ClientEventTable) ProcessServerMetadataModificationEvent(
	ctx context.Context,
	config_obj *config_proto.Config,
	event *ordereddict.Dict) {

	// Only trigger on server metadata changes
	client_id, pres := event.GetString("client_id")
	if !pres || client_id != "server" {
		return
	}

	logger := logging.GetLogger(self.config_obj, &logging.FrontendComponent)
	logger.Debug("<green>client_monitoring</>: Reloading table because server metadata was updated")

	notifier, err := services.GetNotifier(config_obj)
	if err == nil {
		notifier.NotifyDirectListener(loadFileQueue(config_obj))
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
	logger.Debug("<green>Updating Client Event Table</> because %v was updated", modified_name)

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
		notifier, err := services.GetNotifier(config_obj)
		if err == nil {
			notifier.NotifyDirectListener(loadFileQueue(config_obj))
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

	if config_obj.Frontend == nil {
		return errors.New("Frontend not configured")
	}

	// Do not hold the lock while we compile event table from file -
	// it can take some time and this is the critical path.
	state, err := self.load_from_file(ctx, config_obj)
	if err != nil {
		return err
	}

	self.mu.Lock()
	defer self.mu.Unlock()

	self.state = state
	return nil
}

func (self *ClientEventTable) load_from_file(
	ctx context.Context, config_obj *config_proto.Config) (
	*flows_proto.ClientEventTable, error) {

	logger := logging.GetLogger(config_obj, &logging.FrontendComponent)
	logger.Debug("Reloading client monitoring tables from datastore\n")
	db, err := datastore.GetDB(config_obj)
	if err != nil {
		return nil, err
	}

	state := &flows_proto.ClientEventTable{}
	err = db.GetSubject(config_obj, paths.ClientMonitoringFlowURN, state)
	if err != nil || state.Version == 0 {
		// No client monitoring rules found, install some
		// defaults.
		state.Artifacts = &flows_proto.ArtifactCollectorArgs{
			Artifacts: config_obj.Frontend.DefaultClientMonitoringArtifacts,
		}
		state.LabelEvents = append(state.LabelEvents,
			&flows_proto.LabelEvents{
				Label: "Quarantine",
				Artifacts: &flows_proto.ArtifactCollectorArgs{
					Artifacts: []string{
						"Windows.Remediation.QuarantineMonitor",
					},
				},
			})
		logger.Info("Creating default Client Monitoring Service")

		err = compileState(ctx, config_obj, state)
		if err != nil {
			return nil, err
		}

		state.Version = uint64(utils.GetTime().Now().UnixNano())

		return state, self.setClientMonitoringState(ctx, config_obj, "", state)
	}

	// Update the new version
	state.Version = uint64(utils.GetTime().Now().UnixNano())

	clear_caches(state)
	return state, compileState(ctx, config_obj, state)
}

// We need to re-load the event table from disk whenever the artifact
// definitions are modified. Sometimes we modify a lot of artifacts
// very quickly (e.g. in administrative tasks). But reloading for each
// artifact does not really help much. In this case we want to limit
// the rate at which we reload the event table from disk and debounce
// this. The below code should limit how quickly we are notified of
// changes in the event tables. We will be eventually notified but
// hopefully only a few times instead of for each modification.
func (self *ClientEventTable) startLoadFileLoop(
	ctx context.Context,
	wg *sync.WaitGroup, config_obj *config_proto.Config) error {
	defer wg.Done()

	notifier, err := services.GetNotifier(config_obj)
	if err != nil {
		return err
	}

	// Debounce file load through the notifier.
	notification, remove := notifier.ListenForNotification(loadFileQueue(config_obj))
	defer remove()

	for {
		select {
		case <-ctx.Done():
			return nil

			// Debounced file load notification.
		case <-notification:
			err := self.LoadFromFile(ctx, config_obj)
			if err != nil {
				logger := logging.GetLogger(
					config_obj, &logging.FrontendComponent)
				logger.Error("self.startLoadFileLoop: %v", err)
			}
			remove()

			notification, remove = notifier.ListenForNotification(
				loadFileQueue(config_obj))
		}
	}
}

// Main loop.
func (self *ClientEventTable) Start(
	ctx context.Context,
	wg *sync.WaitGroup, config_obj *config_proto.Config) (err error) {

	defer wg.Done()

	logger := logging.GetLogger(config_obj, &logging.FrontendComponent)
	logger.Info("<green>Starting</> Client Monitoring Service for %v",
		services.GetOrgName(config_obj))
	journal, err := services.GetJournal(config_obj)
	if err != nil {
		return err
	}

	wg.Add(1)
	go func() {
		err1 := self.startLoadFileLoop(ctx, wg, config_obj)
		if err1 != nil && err == nil {
			err = err1
		}
	}()

	events, cancel := journal.Watch(
		ctx, "Server.Internal.ArtifactModification",
		"client_monitoring_service")
	defer cancel()

	metadata_mod_event, metadata_mod_event_cancel := journal.Watch(
		ctx, "Server.Internal.MetadataModifications",
		"client_monitoring_service")
	defer metadata_mod_event_cancel()

	for {
		select {
		case <-ctx.Done():
			return nil

		case event, ok := <-metadata_mod_event:
			if !ok {
				return nil
			}
			self.ProcessServerMetadataModificationEvent(
				ctx, config_obj, event)

		case event, ok := <-events:
			if !ok {
				return nil
			}
			self.ProcessArtifactModificationEvent(ctx, config_obj, event)
		}
	}
}

// Runs at frontend start to initialize the client monitoring table.
func NewClientMonitoringService(
	ctx context.Context,
	wg *sync.WaitGroup,
	config_obj *config_proto.Config) (services.ClientEventTable, error) {

	event_table := &ClientEventTable{
		id:         uuid.New().String(),
		config_obj: config_obj,
	}

	wg.Add(1)
	go func() {
		err := event_table.Start(ctx, wg, config_obj)
		if err != nil {
			logger := logging.GetLogger(config_obj, &logging.FrontendComponent)
			logger.Error("NewClientMonitoringService: %v", err)
		}
	}()

	return event_table, event_table.LoadFromFile(ctx, config_obj)
}

func loadFileQueue(config_obj *config_proto.Config) string {
	return "ClientMonitoring" + config_obj.OrgId
}
