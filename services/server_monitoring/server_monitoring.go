// Just like client monitoring, the server can run multiple event VQL
// queries at the same time. These are typically used to watch for
// various events on the server and respond to them.

package server_monitoring

import (
	"context"
	"sync"

	"github.com/Velocidex/ordereddict"
	"google.golang.org/protobuf/proto"
	actions_proto "www.velocidex.com/golang/velociraptor/actions/proto"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	"www.velocidex.com/golang/velociraptor/constants"
	"www.velocidex.com/golang/velociraptor/datastore"
	"www.velocidex.com/golang/velociraptor/file_store"
	"www.velocidex.com/golang/velociraptor/file_store/result_sets"
	flows_proto "www.velocidex.com/golang/velociraptor/flows/proto"
	"www.velocidex.com/golang/velociraptor/logging"
	"www.velocidex.com/golang/velociraptor/paths/artifacts"
	"www.velocidex.com/golang/velociraptor/services"
	"www.velocidex.com/golang/velociraptor/utils"
	vql_subsystem "www.velocidex.com/golang/velociraptor/vql"
	"www.velocidex.com/golang/vfilter"
)

var (
	DefaultServerMonitoringTable = flows_proto.ArtifactCollectorArgs{
		Artifacts: []string{"Server.Monitor.Health"},
	}
)

type EventTable struct {
	mu sync.Mutex

	config_obj *config_proto.Config

	cancel func()

	wg *sync.WaitGroup

	Clock utils.Clock
}

func (self *EventTable) Close() {
	self.mu.Lock()
	defer self.mu.Unlock()

	// Close the old table.
	if self.cancel != nil {
		self.cancel()
	}

	// Wait here until all the old queries are cancelled.
	self.wg.Wait()
}

func (self *EventTable) Update(
	config_obj *config_proto.Config,
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
		false, /* should_obfuscate */
		request)
	if err != nil {
		return err
	}

	// Now store the monitoring table on disk.
	db, err := datastore.GetDB(config_obj)
	if err != nil {
		return err
	}

	err = db.SetSubject(config_obj, constants.ServerMonitoringFlowURN, request)
	if err != nil {
		return err
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

	builder := services.ScopeBuilder{
		Config: config_obj,
		// Disable ACLs on the client.
		ACLManager: vql_subsystem.NullACLManager{},
		Env:        ordereddict.NewDict(),
		Repository: repository,
		Logger:     logging.NewPlainLogger(config_obj, &logging.FrontendComponent),
	}

	for _, env_spec := range vql_request.Env {
		builder.Env.Set(env_spec.Key, env_spec.Value)
	}

	scope := manager.BuildScope(builder)
	/*	vfilter.InstallThrottler(
		scope, vfilter.NewTimeThrottler(float64(50)))
	*/
	// Create a result set writer for the output.
	opts := vql_subsystem.EncOptsFromScope(scope)
	file_store_factory := file_store.GetFileStore(config_obj)

	artifact_name := getArtifactName(vql_request)
	scope.Log("server_monitoring: Collecting %v", artifact_name)

	path_manager := artifacts.NewArtifactPathManager(
		config_obj, "", "", artifact_name)
	path_manager.Clock = self.Clock

	rs_writer, err := result_sets.NewResultSetWriter(
		file_store_factory, path_manager, opts, false /* truncate */)
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
			vql, err := vfilter.Parse(query.VQL)
			if err != nil {
				scope.Log("server_monitoring: %v", err)
				return
			}

			eval_chan := vql.Eval(ctx, scope)

		one_query:
			for {
				select {
				case <-ctx.Done():
					return

				case row, ok := <-eval_chan:
					if !ok {
						break one_query
					}

					rs_writer.Write(vfilter.RowToDict(ctx, scope, row).
						Set("_ts", self.Clock.Now().Unix()))
					rs_writer.Flush()

				}
			}
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
		config_obj,
		constants.ServerMonitoringFlowURN,
		artifacts)
	if err != nil || artifacts.Artifacts == nil {
		// No monitoring rules found, set defaults.
		artifacts = proto.Clone(
			&DefaultServerMonitoringTable).(*flows_proto.ArtifactCollectorArgs)
	}

	logger := logging.GetLogger(
		config_obj, &logging.FrontendComponent)
	logger.Info("server_monitoring: <green>Starting</> Server Monitoring Service")

	manager := &EventTable{
		config_obj: config_obj,
		wg:         &sync.WaitGroup{},
		Clock:      utils.RealClock{},
	}

	wg.Add(1)
	go func() {
		defer wg.Done()
		defer services.RegisterServerEventManager(nil)

		<-ctx.Done()
	}()

	services.RegisterServerEventManager(manager)

	return manager.Update(config_obj, artifacts)
}
