package services

import (
	"context"
	"fmt"
	"log"
	"runtime/debug"
	"sync"
	"time"

	"github.com/Velocidex/ordereddict"
	"github.com/pkg/errors"
	"www.velocidex.com/golang/velociraptor/artifacts"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	crypto_proto "www.velocidex.com/golang/velociraptor/crypto/proto"
	"www.velocidex.com/golang/velociraptor/datastore"
	"www.velocidex.com/golang/velociraptor/file_store"
	"www.velocidex.com/golang/velociraptor/file_store/api"
	flows_proto "www.velocidex.com/golang/velociraptor/flows/proto"
	"www.velocidex.com/golang/velociraptor/logging"
	"www.velocidex.com/golang/velociraptor/paths"
	"www.velocidex.com/golang/velociraptor/result_sets"
	"www.velocidex.com/golang/velociraptor/utils"
	vql_subsystem "www.velocidex.com/golang/velociraptor/vql"
	"www.velocidex.com/golang/vfilter"
)

type serverLogger struct {
	config_obj *config_proto.Config
	w          *result_sets.ResultSetWriter
}

func (self *serverLogger) Write(b []byte) (int, error) {
	msg := artifacts.DeobfuscateString(self.config_obj, string(b))
	self.w.Write(ordereddict.NewDict().
		Set("Timestamp", fmt.Sprintf("%v", time.Now().UTC().UnixNano()/1000)).
		Set("time", time.Now().UTC().String()).
		Set("message", msg))
	return len(b), nil
}

type ServerArtifactsRunner struct {
	config_obj *config_proto.Config
	mu         sync.Mutex
	timeout    time.Duration
}

func (self *ServerArtifactsRunner) Start(
	ctx context.Context,
	wg *sync.WaitGroup) {
	defer wg.Done()

	logger := logging.GetLogger(
		self.config_obj, &logging.FrontendComponent)

	// Listen for notifications from the server.
	notification := ListenForNotification("server")
	defer NotifyListener(self.config_obj, "server")

	self.process(ctx, wg)

	for {
		select {
		// Check the queues anyway every minute in case we miss the
		// notification.
		case <-time.After(time.Duration(60) * time.Second):
			self.process(ctx, wg)

		case <-ctx.Done():
			return

		case quit := <-notification:
			if quit {
				logger.Info("ServerArtifactsRunner: quit.")
				return
			}
			err := self.process(ctx, wg)
			if err != nil {
				logger.Error("ServerArtifactsRunner: %v", err)
				return
			}

			// Listen again.
			notification = ListenForNotification("server")
		}
	}
}

func (self *ServerArtifactsRunner) process(
	ctx context.Context,
	wg *sync.WaitGroup) error {

	logger := logging.GetLogger(
		self.config_obj, &logging.FrontendComponent)

	db, err := datastore.GetDB(self.config_obj)
	if err != nil {
		return err
	}

	tasks, err := db.GetClientTasks(self.config_obj, "server", true)
	if err != nil {
		return err
	}

	wg.Add(1)
	defer func() {
		defer wg.Done()

		for _, task := range tasks {
			err := self.processTask(ctx, task)
			if err != nil {
				logger.Error("ServerArtifactsRunner: %v", err)
			}
		}
	}()

	return nil
}

func (self *ServerArtifactsRunner) processTask(
	ctx context.Context,
	task *crypto_proto.GrrMessage) error {

	flow_urn := paths.NewFlowPathManager("server", task.SessionId).Path()
	collection_context := &flows_proto.ArtifactCollectorContext{}
	db, err := datastore.GetDB(self.config_obj)
	if err != nil {
		return err
	}

	err = db.GetSubject(self.config_obj, flow_urn, collection_context)
	if err != nil {
		return err
	}

	db.UnQueueMessageForClient(self.config_obj, "server", task)

	self.runQuery(ctx, task, collection_context)

	collection_context.State = flows_proto.ArtifactCollectorContext_TERMINATED
	collection_context.ActiveTime = uint64(time.Now().UnixNano() / 1000)
	return db.SetSubject(self.config_obj, flow_urn, collection_context)
}

func (self *ServerArtifactsRunner) runQuery(
	ctx context.Context,
	task *crypto_proto.GrrMessage,
	collection_context *flows_proto.ArtifactCollectorContext) error {
	self.mu.Lock()
	defer self.mu.Unlock()

	flow_id := task.SessionId

	// Set up the logger for writing query logs. Note this must be
	// destroyed last since we need to be able to receive logs
	// from scope destructors.
	path_manager := paths.NewFlowPathManager("server", flow_id).Log()
	rs_writer, err := result_sets.NewResultSetWriter(
		self.config_obj, path_manager, false /* truncate */)
	if err != nil {
		return err
	}
	defer rs_writer.Close()

	arg := task.VQLClientAction
	if arg == nil {
		return errors.New("VQLClientAction should be specified.")
	}

	if arg.Query == nil {
		return errors.New("Query should be specified")
	}

	// Cancel the query after this deadline
	deadline := time.After(self.timeout)
	started := time.Now().Unix()
	sub_ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	// Server artifacts run with full access. In order to collect
	// them in the first place we need COLLECT_SERVER permissions.
	scope := artifacts.ScopeBuilder{
		Config: self.config_obj,

		// For server artifacts, upload() ends up writing in
		// the file store. NOTE: This allows arbitrary
		// filestore write. Using this we can manager the
		// files in the filestore using VQL artifacts.
		Uploader: api.NewFileStoreUploader(
			self.config_obj,
			file_store.GetFileStore(self.config_obj), "/"),
		ACLManager: vql_subsystem.NewRoleACLManager("administrator"),
		Logger: log.New(
			&serverLogger{self.config_obj, rs_writer},
			"server", 0),
	}.Build()
	defer scope.Close()

	env := ordereddict.NewDict()
	for _, env_spec := range arg.Env {
		env.Set(env_spec.Key, env_spec.Value)
	}
	scope.AppendVars(env)

	// If we panic below we need to recover and report this to the
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
		var rs_writer *result_sets.ResultSetWriter
		if query.Name != "" {
			name := artifacts.DeobfuscateString(
				self.config_obj, query.Name)

			path_manager := result_sets.NewArtifactPathManager(
				self.config_obj, "server", flow_id, name)
			rs_writer, err = result_sets.NewResultSetWriter(
				self.config_obj, path_manager, false /* truncate */)
			defer rs_writer.Close()

			// Update the artifacts with results in the
			// context.
			if !utils.InString(
				collection_context.ArtifactsWithResults, name) {
				collection_context.ArtifactsWithResults = append(
					collection_context.ArtifactsWithResults, name)
			}
		}

		row_idx := 0

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
				if rs_writer != nil {
					row_idx += 1
					rs_writer.Write(vfilter.RowToDict(
						sub_ctx, scope, row))
				}
			}
		}

		if query.Name != "" {
			scope.Log("Query %v: Emitted %v rows", query.Name, row_idx)
		}
	}

	return nil
}

func startServerArtifactService(
	ctx context.Context,
	wg *sync.WaitGroup,
	config_obj *config_proto.Config) error {

	result := &ServerArtifactsRunner{
		config_obj: config_obj,
		timeout:    time.Second * time.Duration(600),
	}

	logger := logging.GetLogger(
		config_obj, &logging.FrontendComponent)
	logger.Info("Starting Server Artifact Runner Service")

	wg.Add(1)
	go result.Start(ctx, wg)

	return nil
}
