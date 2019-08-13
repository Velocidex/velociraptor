package services

import (
	"context"
	"fmt"
	"path"
	"runtime/debug"
	"sync"
	"time"

	"github.com/golang/protobuf/proto"
	"github.com/pkg/errors"
	actions_proto "www.velocidex.com/golang/velociraptor/actions/proto"
	"www.velocidex.com/golang/velociraptor/artifacts"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	crypto_proto "www.velocidex.com/golang/velociraptor/crypto/proto"
	"www.velocidex.com/golang/velociraptor/datastore"
	"www.velocidex.com/golang/velociraptor/file_store"
	"www.velocidex.com/golang/velociraptor/file_store/csv"
	flows_proto "www.velocidex.com/golang/velociraptor/flows/proto"
	"www.velocidex.com/golang/velociraptor/logging"
	"www.velocidex.com/golang/velociraptor/notifications"
	vql_subsystem "www.velocidex.com/golang/velociraptor/vql"
	"www.velocidex.com/golang/vfilter"
)

const (
	source = "server"
)

type ServerArtifactsRunner struct {
	config_obj *config_proto.Config
	mu         sync.Mutex
	Done       chan bool
	Scopes     []*vfilter.Scope
	notifier   *notifications.NotificationPool
	wg         sync.WaitGroup

	timeout time.Duration
}

func (self *ServerArtifactsRunner) Close() {
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
}

func (self *ServerArtifactsRunner) Start() {
	logger := logging.GetLogger(
		self.config_obj, &logging.FrontendComponent)

	// Listen for notifications from the server.
	notification, _ := self.notifier.Listen(source)
	defer self.notifier.Notify(source)

	self.process()

	for {
		select {
		case <-self.Done:
			return

			// Check the queues anyway every minute in case we miss the
			// notification.
		case <-time.After(time.Duration(60) * time.Second):
			self.process()

		case quit := <-notification:
			if quit {
				logger.Info("ServerArtifactsRunner: quit.")
				return
			}
			err := self.process()
			if err != nil {
				logger.Error("ServerArtifactsRunner: %v", err)
				return
			}

			// Listen again.
			notification, _ = self.notifier.Listen(source)
		}
	}
}

func (self *ServerArtifactsRunner) process() error {
	self.wg.Add(1)
	defer self.wg.Done()

	logger := logging.GetLogger(
		self.config_obj, &logging.FrontendComponent)

	db, err := datastore.GetDB(self.config_obj)
	if err != nil {
		return err
	}

	tasks, err := db.GetClientTasks(self.config_obj, source, true)
	if err != nil {
		return err
	}

	for _, task := range tasks {
		err := self.processTask(task)
		if err != nil {
			logger.Error("ServerArtifactsRunner: %v", err)
		}
	}

	return nil
}

func (self *ServerArtifactsRunner) processTask(task *crypto_proto.GrrMessage) error {
	flow_id := path.Base(task.SessionId)
	flow_path := path.Join("/clients/server/flows/", flow_id)

	db, err := datastore.GetDB(self.config_obj)
	if err != nil {
		return err
	}

	flow_obj := &flows_proto.AFF4FlowObject{}
	err = db.GetSubject(self.config_obj, flow_path, flow_obj)
	if err != nil {
		return err
	}

	db.UnQueueMessageForClient(self.config_obj, source, task)

	self.runQuery(task)

	flow_obj.FlowContext.State = flows_proto.FlowContext_TERMINATED
	flow_obj.FlowContext.ActiveTime = uint64(time.Now().UnixNano() / 1000)
	return db.SetSubject(self.config_obj, flow_path, flow_obj)
}

func (self *ServerArtifactsRunner) runQuery(
	task *crypto_proto.GrrMessage) error {
	self.mu.Lock()
	defer self.mu.Unlock()

	arg, err := ExtractVQLCollectorArgs(task)
	if err != nil {
		return err
	}

	if arg.Query == nil {
		return errors.New("Query should be specified")
	}

	flow_id := path.Base(task.SessionId)

	// Cancel the query after this deadline
	deadline := time.After(self.timeout)
	started := time.Now().Unix()
	sub_ctx, cancel := context.WithCancel(context.Background())

	// Cancel the context when the cancel channel is closed.
	go func() {
		<-self.Done
		cancel()
	}()

	env := vfilter.NewDict().
		Set("server_config", self.config_obj).
		Set("config", self.config_obj.Client).
		Set(vql_subsystem.CACHE_VAR, vql_subsystem.NewScopeCache())

	for _, env_spec := range arg.Env {
		env.Set(env_spec.Key, env_spec.Value)
	}

	repository, err := artifacts.GetGlobalRepository(self.config_obj)
	if err != nil {
		return err
	}
	scope := artifacts.MakeScope(repository).AppendVars(env)
	scope.Logger = logging.NewPlainLogger(
		self.config_obj, &logging.FrontendComponent)

	self.Scopes = append(self.Scopes, scope)

	// If we panic we need to recover and report this to the
	// server.
	defer func() {
		r := recover()
		if r != nil {
			scope.Log(string(debug.Stack()))
		}
	}()

	// All the queries will use the same scope. This allows one
	// query to define functions for the next query in order.
	for _, query := range arg.Query {
		vql, err := vfilter.Parse(query.VQL)
		if err != nil {
			return err
		}

		read_chan := vql.Eval(sub_ctx, scope)

		var write_chan chan vfilter.Row
		if query.Name != "" {
			name := artifacts.DeobfuscateString(
				self.config_obj, query.Name)
			artifact_name, source_name := artifacts.
				QueryNameToArtifactAndSource(name)

			log_path := artifacts.GetCSVPath(
				/* client_id */ source,
				"",
				/* flow_id */ flow_id,
				artifact_name, source_name,
				artifacts.MODE_SERVER)
			write_chan = self.GetWriter(scope, log_path)
			defer close(write_chan)
		}

	process_query:
		for {
			select {
			case <-deadline:
				msg := fmt.Sprintf("Query timed out after %v seconds",
					time.Now().Unix()-started)
				scope.Log(msg)

				// Cancel the sub ctx but do not exit
				// - we need to wait for the sub query
				// to finish after cancelling so we
				// can at least return any data it
				// has.
				cancel()

				// Try again after a while to prevent spinning here.
				deadline = time.After(self.timeout)

			case row, ok := <-read_chan:
				if !ok {
					break process_query
				}
				if write_chan != nil {
					write_chan <- row
				}
			}
		}
	}

	return nil
}

func (self *ServerArtifactsRunner) GetWriter(
	scope *vfilter.Scope,
	log_path string) chan vfilter.Row {

	logger := logging.GetLogger(
		self.config_obj, &logging.FrontendComponent)

	row_chan := make(chan vfilter.Row)

	go func() {
		var columns []string

		file_store_factory := file_store.GetFileStore(self.config_obj)

		fd, err := file_store_factory.WriteFile(log_path)
		if err != nil {
			logger.Error("Error: %v\n", err)
			return
		}

		writer, err := csv.GetCSVWriter(scope, fd)
		if err != nil {
			logger.Error("Error: %v\n", err)
			return
		}
		defer writer.Close()

		for row := range row_chan {
			if columns == nil {
				columns = scope.GetMembers(row)
			}

			// First column is a row timestamp. This makes
			// it easier to do a row scan for time ranges.
			dict_row := vfilter.NewDict()
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

// Unpack the GrrMessage payload. The return value should be type asserted.
func ExtractVQLCollectorArgs(message *crypto_proto.GrrMessage) (
	*actions_proto.VQLCollectorArgs, error) {
	if message.ArgsRdfName != "VQLCollectorArgs" {
		return nil, errors.New("Unknown message - expected VQLCollectorArgs")
	}

	result := &actions_proto.VQLCollectorArgs{}
	err := proto.Unmarshal(message.Args, result)
	if err != nil {
		return nil, err
	}
	return result, nil
}

func startServerArtifactService(config_obj *config_proto.Config,
	notifier *notifications.NotificationPool) (

	*ServerArtifactsRunner, error) {
	result := &ServerArtifactsRunner{
		config_obj: config_obj,
		Done:       make(chan bool),
		notifier:   notifier,
		timeout:    time.Second * time.Duration(600),
	}

	logger := logging.GetLogger(
		config_obj, &logging.FrontendComponent)
	logger.Info("Starting Server Artifact Runner Service")

	go result.Start()
	return result, nil
}
