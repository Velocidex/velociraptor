// Just like client monitoring, the server can run multiple event VQL
// queries at the same time. These are typically used to watch for
// various events on the server and respond to them.

package server_monitoring

import (
	"context"
	"fmt"
	"log"
	"sync"
	"time"

	"github.com/Velocidex/ordereddict"
	"google.golang.org/protobuf/proto"
	"www.velocidex.com/golang/velociraptor/actions"
	actions_proto "www.velocidex.com/golang/velociraptor/actions/proto"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	"www.velocidex.com/golang/velociraptor/constants"
	"www.velocidex.com/golang/velociraptor/datastore"
	flows_proto "www.velocidex.com/golang/velociraptor/flows/proto"
	"www.velocidex.com/golang/velociraptor/logging"
	"www.velocidex.com/golang/velociraptor/paths"
	"www.velocidex.com/golang/velociraptor/paths/artifact_modes"
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

	tracer *QueryTracer

	request         *flows_proto.ArtifactCollectorArgs
	current_queries []*actions_proto.VQLCollectorArgs
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
	logger := logging.GetLogger(self.config_obj, &logging.FrontendComponent)

	// Close the old table.
	if self.cancel != nil {
		logger.Info("<red>Closing</> Server Monitoring Event table for %v",
			services.GetOrgName(self.config_obj))

		self.cancel()
		self.cancel = nil
	}
}

func (self *EventTable) Get() *flows_proto.ArtifactCollectorArgs {
	self.mu.Lock()
	defer self.mu.Unlock()

	return self._Get()
}

func (self *EventTable) _Get() *flows_proto.ArtifactCollectorArgs {
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
	if !pres || client_id != constants.VELOCIRAPTOR_SERVER_CLIENT_ID {
		return
	}

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

	self.mu.Lock()
	defer self.mu.Unlock()

	// Now store the monitoring table on disk.
	db, err := datastore.GetDB(config_obj)
	if err != nil {
		return err
	}

	err = db.SetSubject(config_obj, paths.ServerMonitoringFlowURN, request)
	if err != nil {
		return err
	}

	self.request = request

	// Update the queries immediately
	return self._StartQueries(config_obj, request)
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

// Restart the queries in the request
func (self *EventTable) RestartQueries(config_obj *config_proto.Config) error {
	self.mu.Lock()
	defer self.mu.Unlock()

	request := self._Get()

	logger := logging.GetLogger(self.config_obj, &logging.FrontendComponent)
	logger.Debug("ServerMonitoring: Restarting %v Queries", len(request.Artifacts))

	return self._StartQueries(config_obj, request)
}

func (self *EventTable) _StartQueries(
	config_obj *config_proto.Config,
	request *flows_proto.ArtifactCollectorArgs) error {

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

	// Make a cancellable context for all the sub VQL queries.
	subctx, cancel := context.WithCancel(self.parent_ctx)

	// Compile the collection request into multiple
	// VQLCollectorArgs - each will be collected in a different
	// goroutine.
	vql_requests, err := launcher.CompileCollectorArgs(
		subctx, config_obj, acl_manager, repository,
		services.CompilerOptions{
			IgnoreMissingArtifacts: true,
		}, request)
	if err != nil {
		cancel()
		return err
	}

	// Check if the queries have changed at all. If not, we skip the
	// update.
	if self.equal(vql_requests) {
		logger := logging.GetLogger(self.config_obj, &logging.FrontendComponent)
		logger.Info("server_monitoring: Skipping table update because queries have not changed.")
		cancel()
		return nil
	}

	// Close the current queries.
	self._Close()

	// Prepare a new run context.
	self.request = request
	self.current_queries = vql_requests

	// Create a new ctx for the new run.
	self.cancel = cancel

	// Run each collection separately in parallel.
	for _, vql_request := range vql_requests {
		err = self._RunQuery(subctx, config_obj, self.wg, vql_request)
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

func (self *EventTable) _RunQuery(
	ctx context.Context,
	config_obj *config_proto.Config,
	wg *sync.WaitGroup,
	vql_request *actions_proto.VQLCollectorArgs) error {

	artifact_name := getArtifactName(vql_request)
	mode, err := artifacts.GetArtifactMode(ctx, config_obj, artifact_name)
	if err != nil {
		return err
	}

	// Be more strict of the type of artifacts we can run since they
	// are running as root.
	if mode != artifact_modes.MODE_SERVER_EVENT {
		return fmt.Errorf(
			"Server monitoring can only run artifacts of type SERVER_EVENT, not %v", mode.String())
	}

	// Server event queries are always running as superuser.
	principal := utils.GetSuperuserName(config_obj)

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

	journal_opts := services.JournalOptions{
		ArtifactName: artifact_name,
		ArtifactType: artifact_modes.MODE_SERVER_EVENT,
		Username:     principal,
	}

	// We write the logs directly to files.
	log_path_manager := artifacts.NewArtifactLogPathManagerWithMode(
		config_obj, constants.VELOCIRAPTOR_SERVER_CLIENT_ID, "",
		artifact_name, artifact_modes.MODE_SERVER_EVENT)

	logger := &serverLogger{
		config_obj:   self.config_obj,
		path_manager: log_path_manager,
		ctx:          ctx,
		artifact:     artifact_name,
		principal:    principal,
	}

	builder := services.ScopeBuilder{
		Config: config_obj,
		// Run the monitoring queries as the server account. If the
		// artifact launches other artifacts then it will indicate the
		// creator was the server.
		ACLManager: acl_managers.NewServerACLManager(
			self.config_obj, principal),
		Env:        ordereddict.NewDict(),
		Repository: repository,
		Logger:     log.New(logger, "", 0),
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

	// Add the queries to tracer immediately - the goroutines below
	// will remove them asynchronously as they are completed.
	for _, query := range vql_request.Query {
		self.tracer.Set(query.VQL)
	}

	wg.Add(1)
	go func() {
		defer wg.Done()
		//defer rs_writer.Close()
		defer scope.Close()

		defer scope.Log("server_monitoring: Finished collecting %v", artifact_name)

		for _, query := range vql_request.Query {
			query_log := actions.QueryLog.AddQuery(query.VQL)

			// Can call multiple times: Cover error paths
			defer query_log.Close()
			defer self.Tracer().Clear(query.VQL)

			query_start := uint64(utils.GetTime().Now().UTC().UnixNano() / 1000)

			vql, err := vfilter.Parse(query.VQL)
			if err != nil {
				return
			}

			eval_chan := vql.Eval(ctx, scope)

		one_query:
			for {
				select {
				case <-ctx.Done():
					return

				case <-time.After(utils.Jitter(
					time.Second * time.Duration(heartbeat))):
					scope.Log("Time %v: %s: Waiting for rows.",
						(uint64(utils.GetTime().Now().UTC().UnixNano()/1000)-
							query_start)/1000000, query.Name)

				case row, ok := <-eval_chan:
					if !ok {
						// Remove this query immediately
						query_log.Close()
						break one_query
					}

					// Add the timestamp to the row - this is the
					// server time when the event was generated.
					event := vfilter.RowToDict(ctx, scope, row).
						Set("_ts", utils.GetTime().Now().Unix())

					// Write event to the journal asynchronously.
					journal.PushRowsToArtifactAsync(
						ctx, config_obj, event, journal_opts)
				}
			}

			// Remove the query from the tracer immediately as well as
			// on exit. It is safe to call multiple times.
			self.Tracer().Clear(query.VQL)
		}
	}()

	return nil
}

func (self *EventTable) startLoadFileLoop(
	ctx context.Context, config_obj *config_proto.Config) error {

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

			err := self.RestartQueries(config_obj)
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

	journal, err := services.GetJournal(config_obj)
	if err != nil {
		return err
	}

	wg.Add(1)
	go func() {
		defer wg.Done()

		err := self.startLoadFileLoop(ctx, config_obj)
		if err != nil {
			logger := logging.GetLogger(config_obj, &logging.FrontendComponent)
			logger.Error("startLoadFileLoop: %v", err)
		}
	}()

	events, cancel := journal.Watch(
		ctx, artifacts.ARTIFACT_MODIFICATION,
		"server_monitoring_service")
	defer cancel()

	metadata_mod_event, metadata_mod_event_cancel := journal.Watch(
		ctx, artifacts.CLIENT_METADATA_MODIFICATION,
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
			Artifacts: append([]string{
				"Server.Monitor.Health",
				"Server.Monitoring.RSSFeeds",
			},
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
		wg:         wg,
		tracer:     NewQueryTracer(),
	}

	wg.Add(1)
	go func() {
		defer wg.Done()
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
