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
	"github.com/sirupsen/logrus"
	"www.velocidex.com/golang/velociraptor/actions"
	actions_proto "www.velocidex.com/golang/velociraptor/actions/proto"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	"www.velocidex.com/golang/velociraptor/datastore"
	"www.velocidex.com/golang/velociraptor/file_store"
	flows_proto "www.velocidex.com/golang/velociraptor/flows/proto"
	"www.velocidex.com/golang/velociraptor/logging"
	"www.velocidex.com/golang/velociraptor/paths"
	"www.velocidex.com/golang/velociraptor/paths/artifacts"
	"www.velocidex.com/golang/velociraptor/result_sets"
	"www.velocidex.com/golang/velociraptor/services"
	"www.velocidex.com/golang/velociraptor/utils"
	vql_subsystem "www.velocidex.com/golang/velociraptor/vql"
	"www.velocidex.com/golang/vfilter"
)

type EventTable struct {
	mu sync.Mutex

	config_obj *config_proto.Config

	cancel func()

	wg *sync.WaitGroup

	logger *serverLogger
	clock  utils.Clock

	tracer *QueryTracer
}

func (self *EventTable) Clock() utils.Clock {
	self.mu.Lock()
	defer self.mu.Unlock()

	return self.clock
}

func (self *EventTable) Tracer() *QueryTracer {
	self.mu.Lock()
	defer self.mu.Unlock()

	return self.tracer
}

func (self *EventTable) SetClock(clock utils.Clock) {
	self.mu.Lock()
	defer self.mu.Unlock()

	self.clock = clock
	if self.logger != nil {
		self.logger.Clock = clock
	}
}

func (self *EventTable) Close() {
	self.mu.Lock()
	defer self.mu.Unlock()

	logger := logging.GetLogger(self.config_obj, &logging.FrontendComponent)
	logger.Info("Closing Server Monitoring Event table")

	// Close the old table.
	if self.cancel != nil {
		self.cancel()
	}

	// Wait here until all the old queries are cancelled.
	self.wg.Wait()
}

func (self *EventTable) Update(
	config_obj *config_proto.Config,
	principal string,
	request *flows_proto.ArtifactCollectorArgs) error {
	self.Close()

	self.mu.Lock()
	defer self.mu.Unlock()

	self.wg = &sync.WaitGroup{}
	self.cancel = nil

	logger := logging.GetLogger(self.config_obj, &logging.FrontendComponent)
	logger.Info("server_monitoring: Updating monitoring table")

	// Compile the ArtifactCollectorArgs into a list of requests.
	launcher, err := services.GetLauncher()
	if err != nil {
		return err
	}

	manager, err := services.GetRepositoryManager()
	if err != nil {
		return err
	}

	repository, err := manager.GetGlobalRepository(config_obj)
	if err != nil {
		return err
	}

	// No ACLs enforced on server events.
	acl_manager := vql_subsystem.NullACLManager{}

	// Make a context for all the VQL queries.
	ctx, cancel := context.WithCancel(context.Background())
	self.cancel = cancel

	// Compile the collection request into multiple
	// VQLCollectorArgs - each will be collected in a different
	// goroutine.
	vql_requests, err := launcher.CompileCollectorArgs(
		ctx, config_obj, acl_manager, repository,
		services.CompilerOptions{
			IgnoreMissingArtifacts: true,
		}, request)
	if err != nil {
		return err
	}

	// Now store the monitoring table on disk.
	db, err := datastore.GetDB(config_obj)
	if err != nil {
		return err
	}

	err = db.SetSubject(config_obj, paths.ServerMonitoringFlowURN, request)
	if err != nil {
		return err
	}

	if principal != "" {
		logging.GetLogger(config_obj, &logging.Audit).
			WithFields(logrus.Fields{
				"user":  principal,
				"state": request,
			}).Info("SetServerMonitoringState")
	}

	// Run each collection separately in parallel.
	for _, vql_request := range vql_requests {

		err = self.RunQuery(ctx, config_obj, self.wg, vql_request)
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

	manager, err := services.GetRepositoryManager()
	if err != nil {
		return err
	}

	repository, err := manager.GetGlobalRepository(config_obj)
	if err != nil {
		return err
	}

	artifact_name := getArtifactName(vql_request)
	path_manager, err := artifacts.NewArtifactPathManager(
		config_obj, "", "", artifact_name)
	if err != nil {
		return err
	}

	path_manager.Clock = self.clock

	// We write the logs to special files.
	log_path_manager, err := artifacts.NewArtifactLogPathManager(
		config_obj, "server", "", artifact_name)
	if err != nil {
		return err
	}
	log_path_manager.Clock = self.clock

	self.logger = &serverLogger{
		config_obj:   self.config_obj,
		path_manager: log_path_manager,
		Clock:        self.clock,
	}

	builder := services.ScopeBuilder{
		Config: config_obj,
		// Disable ACLs on the client.
		ACLManager: vql_subsystem.NullACLManager{},
		Env:        ordereddict.NewDict(),
		Repository: repository,
		Logger:     log.New(self.logger, "", 0),
	}

	for _, env_spec := range vql_request.Env {
		builder.Env.Set(env_spec.Key, env_spec.Value)
	}

	scope := manager.BuildScope(builder)
	vfilter.InstallThrottler(
		scope, vfilter.NewTimeThrottler(float64(50)))

	// Log a heartbeat so we know something is happening.
	heartbeat := vql_request.Heartbeat
	if heartbeat == 0 {
		heartbeat = 120
	}

	// Create a result set writer for the output.
	opts := vql_subsystem.EncOptsFromScope(scope)
	file_store_factory := file_store.GetFileStore(config_obj)

	scope.Log("server_monitoring: Collecting <green>%v</>", artifact_name)

	rs_writer, err := result_sets.NewTimedResultSetWriterWithClock(
		file_store_factory, path_manager, opts, self.clock)
	if err != nil {
		scope.Close()
		return err
	}

	wg.Add(1)
	go func() {
		defer wg.Done()
		defer rs_writer.Close()
		defer scope.Close()
		defer scope.Log("server_monitoring: Finished collecting %v", artifact_name)

		for _, query := range vql_request.Query {
			query_log := actions.QueryLog.AddQuery(query.VQL)
			query_start := uint64(self.Clock().Now().UTC().UnixNano() / 1000)

			// Record the current query.
			self.Tracer().Set(query.VQL)
			defer self.Tracer().Clear(query.VQL)

			vql, err := vfilter.Parse(query.VQL)
			if err != nil {
				return
			}

			eval_chan := vql.Eval(ctx, scope)

		one_query:
			for {
				select {
				case <-ctx.Done():
					query_log.Close()
					return

				case <-time.After(time.Second * time.Duration(heartbeat)):
					scope.Log("Time %v: %s: Waiting for rows.",
						(uint64(self.Clock().Now().UTC().UnixNano()/1000)-
							query_start)/1000000, query.Name)

				case row, ok := <-eval_chan:
					if !ok {
						query_log.Close()
						break one_query
					}

					rs_writer.Write(vfilter.RowToDict(ctx, scope, row).
						Set("_ts", self.Clock().Now().Unix()))
					rs_writer.Flush()
				}
			}
			self.tracer.Clear(query.VQL)
		}

	}()

	return nil
}

// Bring up the server monitoring service.
func StartServerMonitoringService(
	ctx context.Context,
	wg *sync.WaitGroup,
	config_obj *config_proto.Config) error {

	db, err := datastore.GetDB(config_obj)
	if err != nil {
		return err
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
	logger.Info("server_monitoring: <green>Starting</> Server Monitoring Service")

	manager := &EventTable{
		config_obj: config_obj,
		wg:         &sync.WaitGroup{},
		clock:      utils.RealClock{},
		tracer:     NewQueryTracer(),
	}

	wg.Add(1)
	go func() {
		defer wg.Done()
		defer services.RegisterServerEventManager(nil)

		// Shut down all server queries in an orderly fasion
		defer manager.Close()

		<-ctx.Done()
	}()

	services.RegisterServerEventManager(manager)

	return manager.Update(config_obj, "", artifacts)
}
