// Just like client monitoring, the server can run multiple event VQL
// queries at the same time. These are typically used to watch for
// various events on the server and respond to them.

package server_monitoring

import (
	"context"
	"log"
	"sync"
	"time"

	"github.com/Velocidex/ordereddict"
	"google.golang.org/protobuf/proto"
	"www.velocidex.com/golang/velociraptor/actions"
	actions_proto "www.velocidex.com/golang/velociraptor/actions/proto"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	"www.velocidex.com/golang/velociraptor/datastore"
	flows_proto "www.velocidex.com/golang/velociraptor/flows/proto"
	"www.velocidex.com/golang/velociraptor/logging"
	"www.velocidex.com/golang/velociraptor/paths"
	"www.velocidex.com/golang/velociraptor/paths/artifacts"
	"www.velocidex.com/golang/velociraptor/services"
	"www.velocidex.com/golang/velociraptor/utils"
	"www.velocidex.com/golang/velociraptor/vql/acl_managers"
	"www.velocidex.com/golang/vfilter"
)

type EventTable struct {
	mu sync.Mutex

	config_obj *config_proto.Config

	parent_ctx context.Context

	cancel func()

	// Wait for all subqueries to finish using this wg
	wg *sync.WaitGroup

	logger *serverLogger

	tracer *QueryTracer

	request         *flows_proto.ArtifactCollectorArgs
	current_queries []*actions_proto.VQLCollectorArgs
}

func (self *EventTable) Wait() {
	self.wg.Wait()
}

func (self *EventTable) Tracer() *QueryTracer {
	self.mu.Lock()
	defer self.mu.Unlock()

	return self.tracer
}

func (self *EventTable) Close() {
	self.mu.Lock()
	defer self.mu.Unlock()

	self._Close()
}

func (self *EventTable) _Close() {
	// Close the old table.
	if self.cancel != nil {
		logger := logging.GetLogger(self.config_obj, &logging.FrontendComponent)
		logger.Info("<red>Closing</> Server Monitoring Event table for %v",
			services.GetOrgName(self.config_obj))

		self.cancel()
		self.cancel = nil
	}

	// Wait here until all the old queries are cancelled. Do not hold
	// the lock while we are waiting or the event table will be
	// deadlocked.
	wg := self.wg
	self.mu.Unlock()
	wg.Wait()
	self.mu.Lock()

	// Get ready for the next run.
	self.wg = &sync.WaitGroup{}
}

func (self *EventTable) Get() *flows_proto.ArtifactCollectorArgs {
	self.mu.Lock()
	defer self.mu.Unlock()

	if self.request == nil {
		return &flows_proto.ArtifactCollectorArgs{}
	}

	return proto.Clone(self.request).(*flows_proto.ArtifactCollectorArgs)
}

func (self *EventTable) ProcessArtifactModificationEvent(
	ctx context.Context,
	config_obj *config_proto.Config,
	event *ordereddict.Dict) {

	modified_name, pres := event.GetString("artifact")
	if !pres || modified_name == "" {
		return
	}

	logger := logging.GetLogger(self.config_obj, &logging.FrontendComponent)
	logger.Info("server_monitoring: Reloading table because artifact %v was updated",
		modified_name)

	notifier, err := services.GetNotifier(config_obj)
	if err == nil {
		notifier.NotifyDirectListener(loadFileQueue(config_obj))
	}
}

func (self *EventTable) ProcessServerMetadataModificationEvent(
	ctx context.Context,
	config_obj *config_proto.Config,
	event *ordereddict.Dict) {

	client_id, pres := event.GetString("client_id")
	if !pres || client_id != "server" {
		return
	}

	logger := logging.GetLogger(self.config_obj, &logging.FrontendComponent)
	logger.Info("<green>server_monitoring</>: Reloading table because server metadata was updated")

	notifier, err := services.GetNotifier(config_obj)
	if err == nil {
		notifier.NotifyDirectListener(loadFileQueue(config_obj))
	}
}

func (self *EventTable) Update(
	ctx context.Context,
	config_obj *config_proto.Config,
	principal string,
	request *flows_proto.ArtifactCollectorArgs) error {

	logger := logging.GetLogger(self.config_obj, &logging.FrontendComponent)
	if principal != "" {
		err := services.LogAudit(ctx,
			config_obj, principal, "SetServerMonitoringState",
			ordereddict.NewDict().
				Set("user", principal).
				Set("state", request))
		if err != nil {
			logger.Error("EventTable Update SetServerMonitoringState %v %v",
				principal, request)
		}
	}

	logger.Info("<green>server_monitoring</>: Updating monitoring table")

	// Now store the monitoring table on disk.
	db, err := datastore.GetDB(config_obj)
	if err != nil {
		return err
	}

	err = db.SetSubject(config_obj, paths.ServerMonitoringFlowURN, request)
	if err != nil {
		return err
	}

	self.mu.Lock()
	self.request = request
	self.mu.Unlock()

	// Update the queries immediately
	return self.StartQueries(config_obj)
}

// Compare a new set of queries with the current set to see if they
// were changed at all.
func (self *EventTable) equal(events []*actions_proto.VQLCollectorArgs) bool {
	if len(events) != len(self.current_queries) {
		return false
	}

	for i := range self.current_queries {
		lhs := self.current_queries[i]
		rhs := events[i]

		if len(lhs.Query) != len(rhs.Query) {
			return false
		}

		for j := range lhs.Query {
			if !proto.Equal(lhs.Query[j], rhs.Query[j]) {
				return false
			}
		}

		if len(lhs.Env) != len(rhs.Env) {
			return false
		}

		for j := range lhs.Env {
			if !proto.Equal(lhs.Env[j], rhs.Env[j]) {
				return false
			}
		}
	}
	return true
}

// Start the queries in the requewst
func (self *EventTable) StartQueries(config_obj *config_proto.Config) error {

	request := self.Get()

	self.mu.Lock()
	defer self.mu.Unlock()

	// Compile the ArtifactCollectorArgs into a list of requests.
	launcher, err := services.GetLauncher(config_obj)
	if err != nil {
		return err
	}

	manager, err := services.GetRepositoryManager(config_obj)
	if err != nil {
		return err
	}

	repository, err := manager.GetGlobalRepository(config_obj)
	if err != nil {
		return err
	}

	// No ACLs enforced on server events.
	acl_manager := acl_managers.NullACLManager{}

	// Make a context for all the VQL queries.
	subctx, cancel := context.WithCancel(self.parent_ctx)
	defer cancel()

	// Compile the collection request into multiple
	// VQLCollectorArgs - each will be collected in a different
	// goroutine.
	vql_requests, err := launcher.CompileCollectorArgs(
		subctx, config_obj, acl_manager, repository,
		services.CompilerOptions{
			IgnoreMissingArtifacts: true,
		}, request)
	if err != nil {
		return err
	}

	// Check if the queries have changed at all. If not, we skip the
	// update.
	if self.equal(vql_requests) {
		logger := logging.GetLogger(self.config_obj, &logging.FrontendComponent)
		logger.Info("server_monitoring: Skipping table update because queries have not changed.")
		return nil
	}

	// Close the current queries.
	self._Close()

	// Prepare a new run context.
	self.request = request
	self.current_queries = vql_requests

	// Create a new ctx for the new run.
	new_ctx, cancel := context.WithCancel(self.parent_ctx)
	self.cancel = cancel

	// Run each collection separately in parallel.
	for _, vql_request := range vql_requests {
		err = self.RunQuery(new_ctx, config_obj, self.wg, vql_request)
		if err != nil {
			return err
		}
	}

	return nil

}

// Scan the VQLCollectorArgs for a name.
func getArtifactName(vql_request *actions_proto.VQLCollectorArgs) string {
	for _, query := range vql_request.Query {
		if query.Name != "" {
			return query.Name
		}
	}
	return ""
}

func (self *EventTable) RunQuery(
	ctx context.Context,
	config_obj *config_proto.Config,
	wg *sync.WaitGroup,
	vql_request *actions_proto.VQLCollectorArgs) error {

	journal, err := services.GetJournal(config_obj)
	if err != nil {
		return err
	}

	manager, err := services.GetRepositoryManager(config_obj)
	if err != nil {
		return err
	}

	repository, err := manager.GetGlobalRepository(config_obj)
	if err != nil {
		return err
	}

	artifact_name := getArtifactName(vql_request)

	// We write the logs directly to files.
	log_path_manager, err := artifacts.NewArtifactLogPathManager(ctx,
		config_obj, "server", "", artifact_name)
	if err != nil {
		return err
	}
	self.logger = &serverLogger{
		config_obj:   self.config_obj,
		path_manager: log_path_manager,
		ctx:          ctx,
		artifact:     artifact_name,
	}

	builder := services.ScopeBuilder{
		Config: config_obj,
		// Run the monitoring queries as the server account. If the
		// artifact launches other artifacts then it will indicate the
		// creator was the server.
		ACLManager: acl_managers.NewServerACLManager(
			self.config_obj, utils.GetSuperuserName(config_obj)),
		Env:        ordereddict.NewDict(),
		Repository: repository,
		Logger:     log.New(self.logger, "", 0),
	}

	for _, env_spec := range vql_request.Env {
		builder.Env.Set(env_spec.Key, env_spec.Value)
	}

	scope := manager.BuildScope(builder)

	// Log a heartbeat so we know something is happening.
	heartbeat := vql_request.Heartbeat
	if heartbeat == 0 {
		heartbeat = 120
	}

	scope.Log("server_monitoring: Collecting <green>%v</>", artifact_name)

	wg.Add(1)
	go func() {
		defer wg.Done()
		//defer rs_writer.Close()
		defer scope.Close()
		defer scope.Log("server_monitoring: Finished collecting %v", artifact_name)

		for _, query := range vql_request.Query {
			query_log := actions.QueryLog.AddQuery(query.VQL)
			query_start := uint64(utils.GetTime().Now().UTC().UnixNano() / 1000)

			// Record the current query.
			self.Tracer().Set(query.VQL)
			defer self.Tracer().Clear(query.VQL)

			vql, err := vfilter.Parse(query.VQL)
			if err != nil {
				query_log.Close()
				return
			}

			eval_chan := vql.Eval(ctx, scope)

		one_query:
			for {
				select {
				case <-ctx.Done():
					query_log.Close()
					return

				case <-time.After(utils.Jitter(
					time.Second * time.Duration(heartbeat))):
					scope.Log("Time %v: %s: Waiting for rows.",
						(uint64(utils.GetTime().Now().UTC().UnixNano()/1000)-
							query_start)/1000000, query.Name)

				case row, ok := <-eval_chan:
					if !ok {
						query_log.Close()
						break one_query
					}

					// Add the timestamp to the row - this is the
					// server time when the event was generated.
					event := vfilter.RowToDict(ctx, scope, row).
						Set("_ts", utils.GetTime().Now().Unix())

					// Write event to the journal asynchronously.
					journal.PushRowsToArtifactAsync(ctx, config_obj,
						event, artifact_name)
				}
			}
			self.tracer.Clear(query.VQL)
		}

	}()

	return nil
}

func (self *EventTable) startLoadFileLoop(
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
			remove()

			notification, remove = notifier.ListenForNotification(
				loadFileQueue(config_obj))
			err := self.StartQueries(config_obj)
			if err != nil {
				logger := logging.GetLogger(config_obj, &logging.FrontendComponent)
				logger.Error("startLoadFileLoop: %v", err)
			}
		}
	}
}

// Main loop.
func (self *EventTable) Start(
	ctx context.Context,
	wg *sync.WaitGroup, config_obj *config_proto.Config) error {

	defer wg.Done()

	journal, err := services.GetJournal(config_obj)
	if err != nil {
		return err
	}

	wg.Add(1)
	go func() {
		err := self.startLoadFileLoop(ctx, wg, config_obj)
		if err != nil {
			logger := logging.GetLogger(config_obj, &logging.FrontendComponent)
			logger.Error("startLoadFileLoop: %v", err)
		}
	}()

	events, cancel := journal.Watch(
		ctx, "Server.Internal.ArtifactModification",
		"server_monitoring_service")
	defer cancel()

	metadata_mod_event, metadata_mod_event_cancel := journal.Watch(
		ctx, "Server.Internal.MetadataModifications",
		"server_monitoring_service")
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

// Bring up the server monitoring service.
func NewServerMonitoringService(
	ctx context.Context,
	wg *sync.WaitGroup,
	config_obj *config_proto.Config) (services.ServerEventManager, error) {

	db, err := datastore.GetDB(config_obj)
	if err != nil {
		return nil, err
	}

	artifacts := &flows_proto.ArtifactCollectorArgs{}
	err = db.GetSubject(
		config_obj, paths.ServerMonitoringFlowURN, artifacts)
	if err != nil || artifacts.Artifacts == nil {
		// No monitoring rules found, set defaults.
		artifacts = &flows_proto.ArtifactCollectorArgs{
			Artifacts: append([]string{"Server.Monitor.Health"},
				config_obj.Frontend.DefaultServerMonitoringArtifacts...),
		}
	}

	logger := logging.GetLogger(
		config_obj, &logging.FrontendComponent)
	logger.Info("server_monitoring: <green>Starting</> Server Monitoring Service for %v",
		services.GetOrgName(config_obj))

	manager := &EventTable{
		config_obj: config_obj,
		parent_ctx: ctx,
		wg:         &sync.WaitGroup{},
		tracer:     NewQueryTracer(),
	}

	wg.Add(1)
	go func() {
		defer manager.Close()

		err := manager.Start(ctx, wg, config_obj)
		if err != nil {
			logger := logging.GetLogger(config_obj, &logging.FrontendComponent)
			logger.Error("NewServerMonitoringService: %v", err)
		}
	}()

	return manager, manager.Update(ctx, config_obj, "", artifacts)
}

func loadFileQueue(config_obj *config_proto.Config) string {
	return "ServerMonitoring" + config_obj.OrgId
}
