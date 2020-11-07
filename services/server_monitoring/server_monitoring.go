// Just like client monitoring, the server can run multiple event VQL
// queries at the same time. These are typically used to watch for
// various events on the server and respond to them.

package server_monitoring

import (
	"context"
	"errors"
	"sync"
	"time"

	"github.com/Velocidex/ordereddict"
	artifacts_proto "www.velocidex.com/golang/velociraptor/artifacts/proto"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	"www.velocidex.com/golang/velociraptor/constants"
	"www.velocidex.com/golang/velociraptor/datastore"
	"www.velocidex.com/golang/velociraptor/file_store"
	"www.velocidex.com/golang/velociraptor/file_store/result_sets"
	flows_proto "www.velocidex.com/golang/velociraptor/flows/proto"
	"www.velocidex.com/golang/velociraptor/logging"
	"www.velocidex.com/golang/velociraptor/paths/artifacts"
	"www.velocidex.com/golang/velociraptor/services"
	vql_subsystem "www.velocidex.com/golang/velociraptor/vql"
	"www.velocidex.com/golang/vfilter"
)

var (
	GlobalEventTable = &EventTable{
		Done: make(chan bool),
	}

	DefaultServerMonitoringTable = flows_proto.ArtifactCollectorArgs{
		Artifacts:  []string{"Server.Monitor.Health"},
		Parameters: &flows_proto.ArtifactParameters{},
	}
)

type EventTable struct {
	mu sync.Mutex

	// This will be closed to signal we need to abort the current
	// event queries.
	Done   chan bool
	Scopes []*vfilter.Scope

	wg sync.WaitGroup
}

func (self *EventTable) Close() {
	self.mu.Lock()
	defer self.mu.Unlock()

	// Close the old table.
	if self.Done != nil {
		close(self.Done)
	}

	// Wait here until all the old queries are cancelled.
	self.wg.Wait()

	// Clean up.
	for _, scope := range self.Scopes {
		scope.Close()
	}

	self.Scopes = []*vfilter.Scope{}
}

func (self *EventTable) Update(
	config_obj *config_proto.Config,
	arg *flows_proto.ArtifactCollectorArgs) error {
	self.Close()

	self.mu.Lock()
	defer self.mu.Unlock()

	// Now create new queries.
	self.Done = make(chan bool)

	// Make a context for all the VQL queries.
	new_ctx, cancel := context.WithCancel(context.Background())

	// Cancel the context when the cancel channel is closed.
	go func() {
		<-self.Done
		cancel()
	}()

	manager, err := services.GetRepositoryManager()
	if err != nil {
		return err
	}

	repository, err := manager.GetGlobalRepository(config_obj)
	if err != nil {
		return err
	}

	for _, name := range arg.Artifacts {
		artifact, pres := repository.Get(config_obj, name)
		if !pres {
			// If the artifact is custom and was removed
			// then this is not a fatal error. We should
			// issue a warning and move on.
			logger := logging.GetLogger(config_obj, &logging.FrontendComponent)
			logger.Warn("<red>Server Artifact Manager</>: Unknown artifact " +
				name + " please remove it from the server monitoring tables.")
			continue
		}

		// Server monitoring artifacts run with full admin
		// permissions.
		scope := manager.BuildScope(
			services.ScopeBuilder{
				Config:     config_obj,
				ACLManager: vql_subsystem.NewRoleACLManager("administrator"),
				Logger: logging.NewPlainLogger(config_obj,
					&logging.FrontendComponent),
			})

		// Closing the scope is deferred to table close.

		// Build env for this query.
		env := ordereddict.NewDict()

		// First set param default.
		for _, param := range artifact.Parameters {
			env.Set(param.Name, param.Default)
		}

		// Then override with the request environment.
		if arg.Parameters != nil {
			for _, env_spec := range arg.Parameters.Env {
				env.Set(env_spec.Key, env_spec.Value)
			}
		}

		// A new scope for each artifact - but shared scope
		// for all sources.
		scope.AppendVars(env)

		// Make sure we do not consume too many resources.
		vfilter.InstallThrottler(
			scope, vfilter.NewTimeThrottler(float64(50)))

		// Keep track of all the scopes so we can close them.
		self.Scopes = append(self.Scopes, scope)

		// Run each source concurrently.
		for _, source := range artifact.Sources {
			err := self.RunQuery(new_ctx, config_obj,
				scope, name, source)
			if err != nil {
				return err
			}
		}
	}

	return nil
}

func (self *EventTable) RunQuery(
	ctx context.Context,
	config_obj *config_proto.Config,
	scope *vfilter.Scope,
	artifact_name string,
	source *artifacts_proto.ArtifactSource) error {

	// Parse all the source VQL to ensure they are valid before we
	// try to run them.
	vqls, err := vfilter.MultiParse(source.Query)
	if err != nil {
		return err
	}

	for idx, vql := range vqls {
		if (idx < len(vqls)-1 && vql.Let == "") ||
			(idx == len(vqls)-1 && vql.Let != "") {
			return errors.New(
				"Invalid artifact " + artifact_name +
					": All Queries in a source " +
					"must be LET queries, except for the " +
					"final one.")
		}
	}

	self.wg.Add(1)

	go func() {
		defer self.wg.Done()

		if source.Name != "" {
			artifact_name = artifact_name + "/" + source.Name
		}

		logger := logging.GetLogger(config_obj, &logging.FrontendComponent)
		logger.Info("Collecting Server Event Artifact: %s", artifact_name)

		path_manager := artifacts.NewArtifactPathManager(
			config_obj, "", "", artifact_name)

		// Append events to previous ones.
		opts := vql_subsystem.EncOptsFromScope(scope)
		file_store_factory := file_store.GetFileStore(config_obj)
		rs_writer, err := result_sets.NewResultSetWriter(
			file_store_factory, path_manager, opts, false /* truncate */)
		if err != nil {
			logger.Error("NewResultSetWriter: %v", err)
			return
		}
		defer rs_writer.Close()

		for _, vql := range vqls {
			for row := range vql.Eval(ctx, scope) {
				rs_writer.Write(vfilter.RowToDict(ctx, scope, row).
					Set("_ts", time.Now().Unix()))
				rs_writer.Flush()
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

	artifacts := flows_proto.ArtifactCollectorArgs{
		Artifacts:  []string{},
		Parameters: &flows_proto.ArtifactParameters{},
	}
	err = db.GetSubject(
		config_obj,
		constants.ServerMonitoringFlowURN,
		&artifacts)
	if err != nil {
		// No monitoring rules found, set defaults.
		artifacts = DefaultServerMonitoringTable
		err = db.SetSubject(
			config_obj, constants.ServerMonitoringFlowURN, &artifacts)
		if err != nil {
			return err
		}
	}

	logger := logging.GetLogger(
		config_obj, &logging.FrontendComponent)
	logger.Info("<green>Starting</> Server Monitoring Service")

	manager := &EventTable{
		Done: make(chan bool),
	}

	wg.Add(1)
	go func() {
		defer wg.Done()
		defer services.RegisterServerEventManager(nil)

		<-ctx.Done()
	}()

	services.RegisterServerEventManager(manager)

	return manager.Update(config_obj, &artifacts)
}
