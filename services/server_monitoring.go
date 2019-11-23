// Just like client monitoring, the server can run multiple event VQL
// queries at the same time. These are typically used to watch for
// various events on the server and respond to them.

package services

import (
	"context"
	"errors"
	"fmt"
	"path"
	"sync"
	"time"

	"github.com/Velocidex/ordereddict"
	"www.velocidex.com/golang/velociraptor/artifacts"
	artifacts_proto "www.velocidex.com/golang/velociraptor/artifacts/proto"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	"www.velocidex.com/golang/velociraptor/constants"
	"www.velocidex.com/golang/velociraptor/datastore"
	"www.velocidex.com/golang/velociraptor/file_store"
	"www.velocidex.com/golang/velociraptor/file_store/csv"
	flows_proto "www.velocidex.com/golang/velociraptor/flows/proto"
	"www.velocidex.com/golang/velociraptor/logging"
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

	repository, err := artifacts.GetGlobalRepository(config_obj)
	if err != nil {
		return err
	}

	for _, name := range arg.Artifacts {
		artifact, pres := repository.Get(name)
		if !pres {
			return errors.New("Unknown artifact " + name)
		}

		env := ordereddict.NewDict().
			Set("server_config", config_obj).
			Set("config", config_obj.Client).
			Set(vql_subsystem.CACHE_VAR, vql_subsystem.NewScopeCache())

		// First set param default.
		for _, param := range artifact.Parameters {
			env.Set(param.Name, param.Default)
		}

		// Then override with the request environment.
		for _, env_spec := range arg.Parameters.Env {
			env.Set(env_spec.Key, env_spec.Value)
		}

		// A new scope for each artifact - but shared scope
		// for all sources.
		scope := artifacts.MakeScope(repository).AppendVars(env)
		scope.Logger = logging.NewPlainLogger(
			config_obj, &logging.FrontendComponent)

		// Make sure we do not consume too many resources.
		vfilter.InstallThrottler(
			scope, vfilter.NewTimeThrottler(float64(50)))

		// Keep track of all the scopes so we can close them.
		self.Scopes = append(self.Scopes, scope)

		// Run each source concurrently.
		for _, source := range artifact.Sources {
			err := self.RunQuery(
				new_ctx, config_obj,
				scope, name, source)
			if err != nil {
				return err
			}
		}
	}

	return nil
}

func (self *EventTable) GetWriter(
	ctx context.Context,
	config_obj *config_proto.Config,
	scope *vfilter.Scope,
	artifact_name string,
	source_name string) chan vfilter.Row {

	row_chan := make(chan vfilter.Row)

	go func() {
		file_store_factory := file_store.GetFileStore(config_obj)
		last_log := ""
		var err error
		var fd file_store.WriteSeekCloser
		var writer *csv.CSVWriter
		var columns []string

		closer := func() {
			if writer != nil {
				writer.Close()
			}
			if fd != nil {
				fd.Close()
			}

			writer = nil
			fd = nil
		}

		defer closer()

		for row := range row_chan {
			log_path := artifacts.GetCSVPath(
				/* client_id */ "",
				artifacts.GetDayName(),
				/* flow_id */ "",
				artifact_name, source_name,
				artifacts.MODE_SERVER_EVENT)

			// We need to rotate the log file.
			if log_path != last_log {
				closer()

				fd, err = file_store_factory.WriteFile(log_path)
				if err != nil {
					fmt.Printf("Error: %v\n", err)
					continue
				}

				writer, err = csv.GetCSVWriter(scope, fd)
				if err != nil {
					continue
				}
			}

			if columns == nil {
				columns = scope.GetMembers(row)
			}

			// First column is a row timestamp. This makes
			// it easier to do a row scan for time ranges.
			dict_row := ordereddict.NewDict().
				Set("_ts", int(time.Now().Unix()))
			for _, column := range columns {
				value, pres := scope.Associative(row, column)
				if pres {
					dict_row.Set(column, value)
				}
			}

			writer.Write(dict_row)
		}
	}()

	return row_chan
}

func (self *EventTable) RunQuery(
	ctx context.Context,
	config_obj *config_proto.Config,
	scope *vfilter.Scope,
	artifact_name string,
	source *artifacts_proto.ArtifactSource) error {

	// Parse all the source VQL to ensure they are valid before we
	// try to run them.
	vqls := []*vfilter.VQL{}
	for idx, query := range source.Queries {
		vql, err := vfilter.Parse(query)
		if err != nil {
			return err
		}

		if (idx < len(source.Queries)-1 && vql.Let == "") ||
			(idx == len(source.Queries) && vql.Let != "") {
			return errors.New(
				"Invalid artifact: All Queries in a source " +
					"must be LET queries, except for the " +
					"final one.")
		}

		vqls = append(vqls, vql)
	}

	self.wg.Add(1)

	go func() {
		defer self.wg.Done()

		name := artifact_name
		if source.Name != "" {
			name = path.Join(artifact_name, source.Name)
		}

		row_chan := self.GetWriter(ctx, config_obj, scope,
			artifact_name, source.Name)
		defer close(row_chan)

		logger := logging.GetLogger(config_obj, &logging.FrontendComponent)
		logger.Info("Collecting Server Event Artifact: %s", name)

		for _, vql := range vqls {
			for row := range vql.Eval(ctx, scope) {
				row_chan <- row
			}
		}
	}()

	return nil
}

// Bring up the server monitoring service.
func startServerMonitoringService(config_obj *config_proto.Config) (
	*EventTable, error) {
	db, err := datastore.GetDB(config_obj)
	if err != nil {
		return nil, err
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
			return nil, err
		}
	}

	logger := logging.GetLogger(
		config_obj, &logging.FrontendComponent)
	logger.Info("Starting Server Monitoring Service")
	err = GlobalEventTable.Update(config_obj, &artifacts)
	if err != nil {
		return nil, err
	}

	return GlobalEventTable, nil
}
