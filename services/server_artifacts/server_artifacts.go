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
	"github.com/go-errors/errors"
	"www.velocidex.com/golang/velociraptor/actions"
	actions_proto "www.velocidex.com/golang/velociraptor/actions/proto"
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

// The Server Artifact Service is responsible for running server side
// VQL artifacts.

// Currently there is only a single server artifact runner, running on
// the master node.

type ServerArtifactsRunner struct {
	config_obj *config_proto.Config
	mu         sync.Mutex

	ctx context.Context
	wg  *sync.WaitGroup

	// Keep track of currently in flight queries so we can cancel
	// them.
	in_flight_collections map[string]*contextManager
}

// Create a bare ServerArtifactsService without the extra management.
func NewServerArtifactRunner(
	ctx context.Context,
	config_obj *config_proto.Config, wg *sync.WaitGroup) *ServerArtifactsRunner {
	return &ServerArtifactsRunner{
		config_obj:            config_obj,
		in_flight_collections: make(map[string]*contextManager),
		ctx:                   ctx,
		wg:                    wg,
	}
}

// Retrieve any tasks waiting in the queues and evaluate them.
func (self *ServerArtifactsRunner) process(
	ctx context.Context,
	config_obj *config_proto.Config,
	wg *sync.WaitGroup) error {

	client_info_manager, err := services.GetClientInfoManager(config_obj)
	if err != nil {
		return err
	}

	messages, err := client_info_manager.GetClientTasks(ctx, "server")
	if err != nil {
		return err
	}

	// We only support two message types - FlowRequest to evaluate a
	// flow and Cancel to cancel it.
	for _, req := range messages {
		session_id := req.SessionId

		if req.Cancel != nil {
			// This collection is now done, cancel it.
			self.Cancel(session_id)
			return nil
		}

		if req.FlowRequest != nil {
			sub_ctx, cancel := context.WithCancel(ctx)
			collection_context, err := NewCollectionContextManager(
				sub_ctx, self.config_obj, "server", session_id)
			if err != nil {
				return err
			}

			wg.Add(1)
			go func() {
				defer wg.Done()
				defer cancel()
				defer collection_context.Save()

				self.ProcessTask(sub_ctx, config_obj,
					req.SessionId, collection_context, req.FlowRequest)
			}()
		}
	}

	return nil
}

func (self *ServerArtifactsRunner) Cancel(flow_id string) {
	self.mu.Lock()
	defer self.mu.Unlock()

	collection_context, pres := self.in_flight_collections[flow_id]
	if pres {
		collection_context.Cancel()
		delete(self.in_flight_collections, flow_id)
	}
}

// A single FlowRequest may contain many VQLClientActions, each may
// represents a single source to be run in parallel. The artifact
// compiler will decide how to structure the artifact into multiple
// VQLClientActions (e.g. by considering precondition clauses).
func (self *ServerArtifactsRunner) ProcessTask(
	ctx context.Context,
	config_obj *config_proto.Config,
	session_id string,
	collection_context CollectionContextManager,
	req *crypto_proto.FlowRequest) error {

	wg := &sync.WaitGroup{}
	for _, task := range req.VQLClientActions {
		// We expect each source to be run in parallel.
		wg.Add(1)
		go func(task *actions_proto.VQLCollectorArgs) {
			defer wg.Done()

			err := self.runQuery(ctx, session_id, task, collection_context)
			if err != nil {
				logger := collection_context.Logger()
				defer logger.Close()

				if logger != nil {
					logger.Write([]byte("ERROR:" + err.Error()))
				}
			}
		}(task)
	}

	// Wait here for all the queries to exit.
	wg.Wait()

	return nil
}

// Called when each query is completed. Will send the message once for
// the entire flow completion.
func (self *ServerArtifactsRunner) maybeSendCompletionMessage(
	session_id string, collection_context CollectionContextManager) {

	flow_context := collection_context.GetContext()
	if flow_context.State == flows_proto.ArtifactCollectorContext_RUNNING {
		return
	}
	row := ordereddict.NewDict().
		Set("Timestamp", utils.GetTime().Now().UTC().Unix()).
		Set("Flow", flow_context).
		Set("FlowId", flow_context.SessionId).
		Set("ClientId", "server")

	journal, err := services.GetJournal(self.config_obj)
	if err != nil {
		return
	}
	journal.PushRowsToArtifact(self.config_obj,
		[]*ordereddict.Dict{row},
		"System.Flow.Completion", "server", session_id,
	)
}

// Run a single query from the collection in parallel.
func (self *ServerArtifactsRunner) runQuery(
	ctx context.Context,
	session_id string,
	arg *actions_proto.VQLCollectorArgs,
	collection_context CollectionContextManager) (err error) {

	// Send a completion event when the query is finished...
	defer self.maybeSendCompletionMessage(session_id, collection_context)

	names_with_response := make(map[string]bool)

	query_context := collection_context.GetQueryContext(arg)
	defer query_context.Close()

	sub_ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	// Set up the logger for writing query logs. Note this must be
	// destroyed last since we need to be able to receive logs
	// from scope destructors.
	if arg.Query == nil {
		return errors.New("Query should be specified")
	}

	// timeout the entire query if it takes too long. Timeout is
	// specified in the artifact definition and set in the query by
	// the artifact compiler.
	timeout := time.Duration(arg.Timeout) * time.Second
	if arg.Timeout == 0 {
		timeout = time.Minute * 10
	}

	// Cancel the query after this deadline
	started := utils.GetTime().Now()
	deadline := time.After(timeout)

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

	flow_path_manager := paths.NewFlowPathManager("server", session_id)
	scope := manager.BuildScope(services.ScopeBuilder{
		Config: self.config_obj,

		// For server artifacts, upload() ends up writing in the file
		// store. NOTE: This allows arbitrary filestore write. Using
		// this we can manage the files in the filestore using VQL
		// artifacts.
		Uploader: NewServerUploader(self.config_obj, session_id,
			flow_path_manager, query_context),

		// Run this query on behalf of the caller so they are
		// subject to ACL checks
		ACLManager: acl_managers.NewServerACLManager(self.config_obj, principal),
		Logger:     log.New(query_context.Logger(), "", 0),
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

		var rs_writer result_sets.ResultSetWriter
		if query.Name == "" {
			// Drain the query but do not relay any data back. These
			// are normally LET queries.
			for _ = range vql.Eval(sub_ctx, scope) {
			}
			continue
		}

		read_chan := vql.Eval(sub_ctx, scope)

		// Write result set into table with this name
		name := artifacts.DeobfuscateString(self.config_obj, query.Name)

		// Allow query scope to control encoding details.
		opts := vql_subsystem.EncOptsFromScope(scope)

		artifact_path_manager := artifact_paths.NewArtifactPathManagerWithMode(
			self.config_obj, "server", session_id, name, paths.MODE_SERVER)
		file_store_factory := file_store.GetFileStore(self.config_obj)
		rs_writer, err = result_sets.NewResultSetWriter(
			file_store_factory, artifact_path_manager.Path(), opts,
			utils.BackgroundWriter, result_sets.AppendMode)
		if err != nil {
			return err
		}
		defer rs_writer.Close()

		// Flush the result set periodically to ensure rows hit the
		// disk sooner. This keeps the GUI updated and allows viewing
		// partial results.
		flusher_done := ResultSetFlusher(sub_ctx, rs_writer)
		defer flusher_done()

	process_query:
		for {
			select {

			// Timed out! Cancel the query.
			case <-deadline:
				msg := fmt.Sprintf("Query timed out after %v seconds",
					utils.GetTime().Now().Unix()-started.Unix())
				scope.Log(msg)

				// Cancel the sub ctx but do not exit
				// - we need to wait for the sub query
				// to finish after cancelling so we
				// can at least return any data it
				// has.
				cancel()

				// Try again after a while to prevent spinning here.
				deadline = time.After(time.Second)

			// Read some data from the query
			case row, ok := <-read_chan:
				if !ok {
					query_log.Close()
					break process_query
				}

				// rs_writer has its own internal buffering so it is
				// ok to write a row at a time.
				rs_writer.Write(vfilter.RowToDict(sub_ctx, scope, row))
				query_context.UpdateStatus(func(s *crypto_proto.VeloStatus) {
					s.ResultRows++
					_, pres := names_with_response[name]
					if !pres {
						s.NamesWithResponse = append(s.NamesWithResponse, name)
						names_with_response[name] = true
					}
				})
			}
		}
	}

	return nil
}

func NewServerArtifactService(
	ctx context.Context,
	wg *sync.WaitGroup,
	config_obj *config_proto.Config) error {

	self := NewServerArtifactRunner(ctx, config_obj, wg)

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
