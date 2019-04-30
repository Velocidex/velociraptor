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

	actions_proto "www.velocidex.com/golang/velociraptor/actions/proto"
	api_proto "www.velocidex.com/golang/velociraptor/api/proto"
	"www.velocidex.com/golang/velociraptor/artifacts"
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
	GlobalEventTable = &EventTable{}
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
	config_obj *api_proto.Config,
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

	for _, name := range arg.Artifacts.Names {
		vql_collector_args := &actions_proto.VQLCollectorArgs{
			OpsPerSecond: 50,
		}

		artifact, pres := repository.Get(name)
		if !pres {
			return errors.New("Unknown artifact " + name)
		}

		err := repository.Compile(artifact, vql_collector_args)
		if err != nil {
			return err
		}

		env := vfilter.NewDict().
			Set("server_config", config_obj).
			Set("config", config_obj.Client).
			Set(vql_subsystem.CACHE_VAR, vql_subsystem.NewScopeCache())

		for _, param := range artifact.Parameters {
			env.Set(param.Name, param.Default)
		}

		for _, env_spec := range arg.Parameters.Env {
			env.Set(env_spec.Key, env_spec.Value)
		}

		scope := artifacts.MakeScope(repository).AppendVars(env)
		scope.Logger = logging.NewPlainLogger(
			config_obj, &logging.FrontendComponent)

		// Make sure we do not consume too many resources.
		vfilter.InstallThrottler(
			scope, vfilter.NewTimeThrottler(float64(50)))

		self.Scopes = append(self.Scopes, scope)

		self.wg.Add(1)
		go self.RunQuery(
			new_ctx, config_obj,
			scope, name, vql_collector_args)
	}

	return nil
}

func (self *EventTable) GetWriter(
	ctx context.Context,
	config_obj *api_proto.Config,
	scope *vfilter.Scope,
	name string) chan vfilter.Row {
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
			now := time.Now()
			log_path := path.Join(
				"server_artifacts", name,
				fmt.Sprintf("%d-%02d-%02d.csv", now.Year(),
					now.Month(), now.Day()))

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

			dict_row, ok := row.(*vfilter.Dict)
			if !ok {
				dict_row := vfilter.NewDict()
				for _, column := range columns {
					value, pres := scope.Associative(row, column)
					if pres {
						dict_row.Set(column, value)
					}
				}
			}
			writer.Write(dict_row)
		}
	}()

	return row_chan
}

func (self *EventTable) RunQuery(
	ctx context.Context,
	config_obj *api_proto.Config,
	scope *vfilter.Scope,
	name string,
	arg *actions_proto.VQLCollectorArgs) {
	defer self.wg.Done()

	row_chan := self.GetWriter(ctx, config_obj, scope, name)
	logger := logging.GetLogger(config_obj, &logging.FrontendComponent)
	logger.Info("Collecting Server Artifact: %s", name)
	for _, query := range arg.Query {
		vql, err := vfilter.Parse(query.VQL)
		if err != nil {
			logger.Error("Error: %v", err)
			return
		}

		for row := range vql.Eval(ctx, scope) {
			row_chan <- row
		}
	}
}

// Bring up the server monitoring service.
func startServerMonitoringService(config_obj *api_proto.Config) (
	*EventTable, error) {
	db, err := datastore.GetDB(config_obj)
	if err != nil {
		return nil, err
	}

	artifacts := flows_proto.ArtifactCollectorArgs{}
	err = db.GetSubject(
		config_obj,
		constants.ServerMonitoringFlowURN,
		&artifacts)
	if err != nil {
		// No monitoring rules found, nothing to do.
		return GlobalEventTable, nil
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
