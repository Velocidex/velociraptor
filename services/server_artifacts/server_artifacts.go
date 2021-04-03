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
	"www.velocidex.com/golang/velociraptor/datastore"
	"www.velocidex.com/golang/velociraptor/file_store"
	"www.velocidex.com/golang/velociraptor/file_store/result_sets"
	flows_proto "www.velocidex.com/golang/velociraptor/flows/proto"
	"www.velocidex.com/golang/velociraptor/logging"
	"www.velocidex.com/golang/velociraptor/paths"
	artifact_paths "www.velocidex.com/golang/velociraptor/paths/artifacts"
	"www.velocidex.com/golang/velociraptor/services"
	"www.velocidex.com/golang/velociraptor/utils"
	vql_subsystem "www.velocidex.com/golang/velociraptor/vql"
	"www.velocidex.com/golang/vfilter"
)

type contextManager struct {
	context      *flows_proto.ArtifactCollectorContext
	mu           sync.Mutex
	config_obj   *config_proto.Config
	path_manager *paths.FlowPathManager
}

func NewCollectionContext(
	config_obj *config_proto.Config,
	client_id string,
	flow_id string) (*contextManager, error) {

	self := &contextManager{
		config_obj:   config_obj,
		path_manager: paths.NewFlowPathManager(client_id, flow_id),
		context:      &flows_proto.ArtifactCollectorContext{},
	}

	return self, self.Load(self.context)
}

// Starts a go routine which saves the context state so the GUI can monitor progress.
func (self *contextManager) StartRefresh(ctx context.Context) {
	go func() {
		for {
			select {
			case <-ctx.Done():
				self.Save()
				return

			case <-time.After(time.Duration(10) * time.Second):
				// Context is finalized no more modifications are allowed.
				if self.context.State != flows_proto.ArtifactCollectorContext_RUNNING {
					return
				}
				self.Save()
			}
		}
	}()
}

// Allow modification of the context under lock.
func (self *contextManager) Modify(cb func(context *flows_proto.ArtifactCollectorContext)) {
	self.mu.Lock()
	defer self.mu.Unlock()

	cb(self.context)
}

func (self *contextManager) Load(context *flows_proto.ArtifactCollectorContext) error {
	self.mu.Lock()
	defer self.mu.Unlock()

	return self.load(context)
}

func (self *contextManager) load(context *flows_proto.ArtifactCollectorContext) error {
	db, err := datastore.GetDB(self.config_obj)
	if err != nil {
		return err
	}

	err = db.GetSubject(self.config_obj, self.path_manager.Path(), context)
	if err != nil {
		return err
	}

	return nil
}

// Flush the context to disk.
func (self *contextManager) Save() error {
	self.mu.Lock()
	defer self.mu.Unlock()

	// Ignore collections which are not running.
	collection_context := &flows_proto.ArtifactCollectorContext{}
	err := self.load(collection_context)
	if err == nil && collection_context.Request != nil &&
		collection_context.State != flows_proto.ArtifactCollectorContext_RUNNING {
		return nil
	}

	db, err := datastore.GetDB(self.config_obj)
	if err != nil {
		return err
	}
	return db.SetSubject(self.config_obj, self.path_manager.Path(), self.context)
}

type serverLogger struct {
	collection_context *contextManager
	config_obj         *config_proto.Config
	path_manager       *paths.FlowPathManager
}

// Send each log message individually to avoid any buffering - logs
// need to be available immediately.
func (self *serverLogger) Write(b []byte) (int, error) {
	msg := artifacts.DeobfuscateString(self.config_obj, string(b))
	err := file_store.PushRows(self.config_obj,
		self.path_manager, []*ordereddict.Dict{
			ordereddict.NewDict().
				Set("Timestamp", time.Now().UTC().UnixNano()/1000).
				Set("time", time.Now().UTC().String()).
				Set("message", msg)})

	// Increment the log count.
	self.collection_context.Modify(func(context *flows_proto.ArtifactCollectorContext) {
		context.TotalLogs++
	})

	return len(b), err
}

type ServerArtifactsRunner struct {
	config_obj       *config_proto.Config
	timeout          time.Duration
	mu               sync.Mutex
	wg               *sync.WaitGroup
	cancellationPool map[string]func()
}

func (self *ServerArtifactsRunner) process(
	ctx context.Context,
	config_obj *config_proto.Config,
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
			err := self.processTask(ctx, config_obj, task)
			if err != nil {
				logger.Error("ServerArtifactsRunner: %v", err)
			}
		}
	}()

	return nil
}

func (self *ServerArtifactsRunner) cancel(flow_id string) {
	self.mu.Lock()
	defer self.mu.Unlock()

	cancel, pres := self.cancellationPool[flow_id]
	if pres {
		cancel()
		delete(self.cancellationPool, flow_id)
	}
}

func (self *ServerArtifactsRunner) processTask(
	ctx context.Context,
	config_obj *config_proto.Config,
	task *crypto_proto.GrrMessage) error {

	collection_context, err := NewCollectionContext(
		self.config_obj, "server", task.SessionId)
	if err != nil {
		return err
	}

	db, _ := datastore.GetDB(self.config_obj)
	err = db.UnQueueMessageForClient(self.config_obj, "server", task)
	if err != nil {
		return err
	}

	// Cancel the current collection
	if task.Cancel != nil {
		path_manager := paths.NewFlowPathManager("server", task.SessionId).Log()
		err = file_store.PushRows(config_obj,
			path_manager, []*ordereddict.Dict{
				ordereddict.NewDict().
					Set("Timestamp", time.Now().UTC().UnixNano()/1000).
					Set("time", time.Now().UTC().String()).
					Set("message", "Cancelling Query")})

		// This task is now done.
		self.cancel(task.SessionId)

		return err
	}

	// Kick off processing in the background and go back to
	// listening for new tasks. We can then cancel this task
	// later.
	self.wg.Add(1)
	go func() {
		defer self.wg.Done()
		err := self.runQuery(ctx, task, collection_context)
		if err != nil {
			return
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
	}()

	return nil
}

func (self *ServerArtifactsRunner) runQuery(
	ctx context.Context,
	task *crypto_proto.GrrMessage,
	collection_context *contextManager) error {

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

	// Cancel the query after this deadline
	deadline := time.After(self.timeout)
	collection_context.Modify(
		func(context *flows_proto.ArtifactCollectorContext) {
			context.StartTime = uint64(time.Now().UnixNano() / 1000)
		})
	started := time.Now()
	sub_ctx, cancel := context.WithCancel(ctx)

	self.mu.Lock()
	self.cancellationPool[task.SessionId] = cancel
	self.mu.Unlock()

	// Write collection context periodically to disk so the GUI
	// can track progress.
	collection_context.StartRefresh(sub_ctx)

	defer func() {
		self.cancel(task.SessionId)
	}()

	// Where to write the logs.
	path_manager := paths.NewFlowPathManager("server", task.SessionId)

	// Server artifacts run with full access. In order to collect
	// them in the first place we need COLLECT_SERVER permissions.
	manager, err := services.GetRepositoryManager()
	if err != nil {
		return err
	}

	principal := arg.Principal
	if principal == "" {
		principal = "administrator"
	}

	scope := manager.BuildScope(services.ScopeBuilder{
		Config: self.config_obj,

		// For server artifacts, upload() ends up writing in
		// the file store. NOTE: This allows arbitrary
		// filestore write. Using this we can manager the
		// files in the filestore using VQL artifacts.
		Uploader: NewServerUploader(self.config_obj,
			path_manager, collection_context),

		// Run this query on behalf of the caller so they are
		// subject to ACL checks
		ACLManager: vql_subsystem.NewServerACLManager(self.config_obj, principal),
		Logger: log.New(&serverLogger{
			collection_context: collection_context,
			config_obj:         self.config_obj,
			path_manager:       path_manager.Log(),
		}, "", 0),
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
			path_manager := artifact_paths.NewArtifactPathManager(
				self.config_obj, "server", task.SessionId, name)

			file_store_factory := file_store.GetFileStore(self.config_obj)
			rs_writer, err = result_sets.NewResultSetWriter(
				file_store_factory, path_manager, opts, false /* truncate */)
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

func StartServerArtifactService(
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

	notifier := services.GetNotifier()
	if notifier == nil {
		return errors.New("Notifier not configured")
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
