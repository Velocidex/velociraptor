/*

   This service provides a runner to execute artifacts on the server.
   Server artifacts run with high privilege and so only users with the
   COLLECT_SERVER permission can run those.

   Server artifacts are used for administration or information purposes.

*/

package server_artifacts

import (
	"context"
	"fmt"
	"log"
	"runtime/debug"
	"sync"
	"time"

	"github.com/Velocidex/ordereddict"
	"github.com/pkg/errors"
	"www.velocidex.com/golang/velociraptor/actions"
	"www.velocidex.com/golang/velociraptor/artifacts"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	crypto_proto "www.velocidex.com/golang/velociraptor/crypto/proto"
	"www.velocidex.com/golang/velociraptor/file_store"
	flows_proto "www.velocidex.com/golang/velociraptor/flows/proto"
	"www.velocidex.com/golang/velociraptor/logging"
	"www.velocidex.com/golang/velociraptor/paths"
	artifact_paths "www.velocidex.com/golang/velociraptor/paths/artifacts"
	"www.velocidex.com/golang/velociraptor/result_sets"
	"www.velocidex.com/golang/velociraptor/vql/acl_managers"

	"www.velocidex.com/golang/velociraptor/services"
	"www.velocidex.com/golang/velociraptor/utils"
	vql_subsystem "www.velocidex.com/golang/velociraptor/vql"
	"www.velocidex.com/golang/vfilter"
)

type ServerArtifactsRunner struct {
	config_obj       *config_proto.Config
	timeout          time.Duration
	mu               sync.Mutex
	wg               *sync.WaitGroup
	cancellationPool map[string]func()
}

func NewServerArtifactRunner(config_obj *config_proto.Config) *ServerArtifactsRunner {
	return &ServerArtifactsRunner{
		config_obj:       config_obj,
		cancellationPool: make(map[string]func()),
		timeout:          time.Second * time.Duration(600),
		wg:               &sync.WaitGroup{},
	}
}

func (self *ServerArtifactsRunner) process(
	ctx context.Context,
	config_obj *config_proto.Config,
	wg *sync.WaitGroup) error {

	client_info_manager, err := services.GetClientInfoManager(config_obj)
	if err != nil {
		return err
	}

	tasks, err := client_info_manager.GetClientTasks("server")
	if err != nil {
		return err
	}

	wg.Add(1)
	defer func() {
		defer wg.Done()

		// Group tasks by session id so we only create one context
		// per session id
		group := make(map[string][]*crypto_proto.VeloMessage)
		for _, task := range tasks {
			existing, _ := group[task.SessionId]
			existing = append(existing, task)
			group[task.SessionId] = existing
		}

		for session_id, tasks := range group {
			collection_context, err := NewCollectionContextManager(
				ctx, self.config_obj, "server", session_id)
			if err != nil {
				continue
			}
			logger, err := NewServerLogger(
				collection_context, config_obj, session_id)
			if err != nil {
				continue
			}

			for _, task := range tasks {
				err = self.ProcessTask(
					ctx, config_obj, task, collection_context, logger)
				if err != nil {
					logger.Write([]byte(
						fmt.Sprintf("ServerArtifactsRunner: %v", err)))
				}
			}

			collection_context.Close()
			logger.Close()
		}
	}()

	return nil
}

func (self *ServerArtifactsRunner) Cancel(flow_id string) {
	self.mu.Lock()
	defer self.mu.Unlock()

	cancel, pres := self.cancellationPool[flow_id]
	if pres {
		cancel()
		delete(self.cancellationPool, flow_id)
	}
}

func (self *ServerArtifactsRunner) ProcessTask(
	ctx context.Context,
	config_obj *config_proto.Config,
	task *crypto_proto.VeloMessage,
	collection_context CollectionContextManager,
	logger *serverLogger) error {

	// Cancel the current collection
	if task.Cancel != nil {
		journal, err := services.GetJournal(config_obj)
		if err != nil {
			return err
		}

		path_manager := paths.NewFlowPathManager("server", task.SessionId).Log()
		err = journal.AppendToResultSet(config_obj,
			path_manager, []*ordereddict.Dict{
				ordereddict.NewDict().
					Set("Timestamp", time.Now().UTC().UnixNano()/1000).
					Set("time", time.Now().UTC().String()).
					Set("message", "Cancelling Query")})

		// This task is now done.
		self.Cancel(task.SessionId)

		return err
	}

	err := self.runQuery(ctx, task, collection_context, logger)
	if err != nil {
		return err
	}

	collection_context.Modify(func(context *flows_proto.ArtifactCollectorContext) {
		if context.State == flows_proto.ArtifactCollectorContext_RUNNING {
			context.State = flows_proto.ArtifactCollectorContext_FINISHED
		}
		context.ActiveTime = uint64(time.Now().UnixNano() / 1000)
		context.ExecutionDuration = (time.Now().UnixNano()/1000 -
			int64(context.StartTime)) * 1000

	})

	_ = collection_context.Save()

	return nil
}

func (self *ServerArtifactsRunner) runQuery(
	ctx context.Context,
	task *crypto_proto.VeloMessage,
	collection_context CollectionContextManager,
	logger *serverLogger) (err error) {

	// Set up the logger for writing query logs. Note this must be
	// destroyed last since we need to be able to receive logs
	// from scope destructors.
	arg := task.VQLClientAction
	if arg == nil {
		return errors.New("VQLClientAction should be specified.")
	}

	if arg.Query == nil {
		return errors.New("Query should be specified")
	}

	timeout := time.Duration(arg.Timeout) * time.Second
	if timeout == 0 {
		timeout = self.timeout
	}

	// Cancel the query after this deadline
	deadline := time.After(timeout)
	collection_context.Modify(
		func(context *flows_proto.ArtifactCollectorContext) {
			context.StartTime = uint64(time.Now().UnixNano() / 1000)
		})
	started := time.Now()
	sub_ctx, cancel := context.WithCancel(ctx)

	self.mu.Lock()
	self.cancellationPool[task.SessionId] = cancel
	self.mu.Unlock()

	defer func() {
		self.Cancel(task.SessionId)

		// Send a completion event when the query is finished..
		flow_context := collection_context.GetContext()
		row := ordereddict.NewDict().
			Set("Timestamp", time.Now().UTC().Unix()).
			Set("Flow", flow_context).
			Set("FlowId", flow_context.SessionId).
			Set("ClientId", "server")

		journal, err := services.GetJournal(self.config_obj)
		if err != nil {
			return
		}
		journal.PushRowsToArtifact(self.config_obj,
			[]*ordereddict.Dict{row},
			"System.Flow.Completion", "server", flow_context.SessionId,
		)
	}()

	// Where to write the logs.
	path_manager := paths.NewFlowPathManager("server", task.SessionId)

	// Server artifacts run with full access. In order to collect
	// them in the first place we need COLLECT_SERVER permissions.
	manager, err := services.GetRepositoryManager(self.config_obj)
	if err != nil {
		return err
	}

	principal := arg.Principal
	if principal == "" && self.config_obj.Client != nil {
		principal = self.config_obj.Client.PinnedServerName
	}

	scope := manager.BuildScope(services.ScopeBuilder{
		Config: self.config_obj,

		// For server artifacts, upload() ends up writing in the file
		// store. NOTE: This allows arbitrary filestore write. Using
		// this we can manage the files in the filestore using VQL
		// artifacts.
		Uploader: NewServerUploader(self.config_obj,
			path_manager, collection_context),

		// Run this query on behalf of the caller so they are
		// subject to ACL checks
		ACLManager: acl_managers.NewServerACLManager(self.config_obj, principal),
		Logger:     log.New(logger, "", 0),
	})
	defer scope.Close()

	scope.Log("Running query on behalf of user %v", principal)

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

	scope.Log("<green>Starting</> query execution.")

	// All the queries will use the same scope. This allows one
	// query to define functions for the next query in order.
	for _, query := range arg.Query {
		query_log := actions.QueryLog.AddQuery(query.VQL)

		vql, err := vfilter.Parse(query.VQL)
		if err != nil {
			return err
		}

		read_chan := vql.Eval(sub_ctx, scope)
		var rs_writer result_sets.ResultSetWriter
		if query.Name != "" {
			name := artifacts.DeobfuscateString(
				self.config_obj, query.Name)

			opts := vql_subsystem.EncOptsFromScope(scope)
			path_manager, err := artifact_paths.NewArtifactPathManager(
				self.config_obj, "server", task.SessionId, name)
			if err != nil {
				return err
			}

			file_store_factory := file_store.GetFileStore(self.config_obj)
			rs_writer, err = result_sets.NewResultSetWriter(
				file_store_factory, path_manager.Path(),
				opts, utils.BackgroundWriter, result_sets.AppendMode)
			if err != nil {
				return err
			}

			defer rs_writer.Close()

			// Flush the result set periodically to ensure
			// rows hit the disk sooner.
			flusher_done := ResultSetFlusher(ctx, rs_writer)
			defer flusher_done()

			// Update the artifacts with results in the
			// context.
			collection_context.Modify(
				func(context *flows_proto.ArtifactCollectorContext) {
					if !utils.InString(
						context.ArtifactsWithResults, name) {
						context.ArtifactsWithResults = append(
							context.ArtifactsWithResults, name)
					}
				})
		}

		row_idx := 0

	process_query:
		for {
			select {
			case <-deadline:
				msg := fmt.Sprintf("Query timed out after %v seconds",
					time.Now().Unix()-started.Unix())
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
					query_log.Close()
					break process_query
				}
				if rs_writer != nil {
					row_idx += 1
					rs_writer.Write(vfilter.RowToDict(sub_ctx, scope, row))
					collection_context.Modify(
						func(context *flows_proto.ArtifactCollectorContext) {
							context.TotalCollectedRows++
						})
				}
			}
		}

		if query.Name != "" {
			scope.Log("Query %v: Emitted %v rows", query.Name, row_idx)
		}
	}

	return nil
}

func NewServerArtifactService(
	ctx context.Context,
	wg *sync.WaitGroup,
	config_obj *config_proto.Config) error {

	self := &ServerArtifactsRunner{
		config_obj:       config_obj,
		timeout:          time.Second * time.Duration(600),
		wg:               wg,
		cancellationPool: make(map[string]func()),
	}

	logger := logging.GetLogger(
		config_obj, &logging.FrontendComponent)
	logger.Info("<green>Starting</> Server Artifact Runner Service")

	notifier, err := services.GetNotifier(config_obj)
	if err != nil {
		return err
	}

	wg.Add(1)
	go func() {
		defer wg.Done()

		logger := logging.GetLogger(config_obj, &logging.FrontendComponent)

		// Listen for notifications from the server.
		notification, cancel := notifier.ListenForNotification("server")
		defer cancel()

		err := self.process(ctx, config_obj, wg)
		if err != nil {
			logger.Error("ServerArtifactsRunner: %v", err)
			return
		}

		for {
			select {
			// Check the queues anyway every minute in case we miss the
			// notification.
			case <-time.After(time.Duration(60) * time.Second):
				err = self.process(ctx, config_obj, wg)
				if err != nil {
					logger.Error("ServerArtifactsRunner: %v", err)
					continue
				}

			case <-ctx.Done():
				return

			case quit := <-notification:
				if quit {
					logger.Info("ServerArtifactsRunner: quit.")
					return
				}
				err := self.process(ctx, config_obj, wg)
				if err != nil {
					logger.Error("ServerArtifactsRunner: %v", err)
					continue
				}

				// Listen again.
				cancel()
				notification, cancel = notifier.ListenForNotification("server")
			}
		}
	}()

	return nil
}
