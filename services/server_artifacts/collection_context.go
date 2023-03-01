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
	"google.golang.org/protobuf/proto"
	"www.velocidex.com/golang/velociraptor/actions"
	actions_proto "www.velocidex.com/golang/velociraptor/actions/proto"
	"www.velocidex.com/golang/velociraptor/artifacts"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	crypto_proto "www.velocidex.com/golang/velociraptor/crypto/proto"
	"www.velocidex.com/golang/velociraptor/file_store"
	flows_proto "www.velocidex.com/golang/velociraptor/flows/proto"
	"www.velocidex.com/golang/velociraptor/paths"
	artifact_paths "www.velocidex.com/golang/velociraptor/paths/artifacts"
	"www.velocidex.com/golang/velociraptor/result_sets"
	"www.velocidex.com/golang/velociraptor/services"
	"www.velocidex.com/golang/velociraptor/services/launcher"
	"www.velocidex.com/golang/velociraptor/utils"
	vql_subsystem "www.velocidex.com/golang/velociraptor/vql"
	"www.velocidex.com/golang/velociraptor/vql/acl_managers"
	"www.velocidex.com/golang/vfilter"
)

// Manage all queries in the same collection

type contextManager struct {
	config_obj *config_proto.Config
	context    *flows_proto.ArtifactCollectorContext

	mu sync.Mutex

	ctx        context.Context
	cancel     func()
	wg         *sync.WaitGroup
	session_id string

	// All queries in the same collection share the same log file.
	log_writer *counterWriter

	// Always report all query status objects - even when completed.
	query_contexts []QueryContext

	row_limit  int64
	byte_limit int64
}

func NewCollectionContextManager(
	ctx context.Context,
	wg *sync.WaitGroup,
	config_obj *config_proto.Config,
	msg *crypto_proto.VeloMessage,
	collection_context *flows_proto.ArtifactCollectorContext) (
	CollectionContextManager, error) {

	flow_id := collection_context.SessionId
	if msg.FlowRequest == nil {
		return nil, errors.New("Invalid request")
	}

	sub_ctx, cancel := context.WithCancel(ctx)
	log_writer, err := NewServerLogWriter(sub_ctx, config_obj, flow_id)
	if err != nil {
		return nil, err
	}

	row_limit := int64(1000000)
	if msg.FlowRequest.MaxRows > 0 {
		row_limit = int64(msg.FlowRequest.MaxRows)
	}

	byte_limit := int64(1000000000)
	if msg.FlowRequest.MaxUploadBytes > 0 {
		byte_limit = int64(msg.FlowRequest.MaxUploadBytes)
	}

	self := &contextManager{
		config_obj: config_obj,
		context:    collection_context,
		ctx:        sub_ctx,
		cancel:     cancel,
		wg:         &sync.WaitGroup{},
		session_id: collection_context.SessionId,
		log_writer: &counterWriter{ResultSetWriter: log_writer},
		row_limit:  row_limit,
		byte_limit: byte_limit,
	}

	return self, nil
}

func (self *contextManager) ChargeRow() {
	self.mu.Lock()
	defer self.mu.Unlock()

	self.row_limit--
	if self.row_limit <= 0 {
		for _, query_ctx := range self.query_contexts {
			query_ctx.UpdateStatus(func(s *crypto_proto.VeloStatus) {
				s.Status = crypto_proto.VeloStatus_GENERIC_ERROR
				s.ErrorMessage = "Row limit exceeded"
			})
		}

		self.cancel()
	}
}

func (self *contextManager) ChargeBytes(bytes int64) {
	self.mu.Lock()
	defer self.mu.Unlock()

	self.byte_limit -= bytes
	if self.byte_limit <= 0 {
		for _, query_ctx := range self.query_contexts {
			query_ctx.UpdateStatus(func(s *crypto_proto.VeloStatus) {
				s.Status = crypto_proto.VeloStatus_GENERIC_ERROR
				s.ErrorMessage = "Byte limit exceeded"
			})
		}

		self.cancel()
	}
}

// Prepare a new query context for this request.
func (self *contextManager) GetQueryContext(
	query *actions_proto.VQLCollectorArgs) QueryContext {

	self.mu.Lock()
	defer self.mu.Unlock()

	// Get the base name of the artifact
	artifact_name := artifacts.DeobfuscateString(
		self.config_obj, actions.GetQueryName(query.Query))
	base, _ := paths.SplitFullSourceName(artifact_name)

	// Will be done when query is closed.
	self.wg.Add(1)
	result := &queryContext{
		id:         utils.GetId(),
		config_obj: self.config_obj,
		session_id: self.session_id,
		start:      utils.GetTime().Now(),
		wg:         self.wg,
		status: crypto_proto.VeloStatus{
			// Initial state for all running queries - will be changed
			// to either OK or GENERIC_ERROR when the query is done.
			Status:      crypto_proto.VeloStatus_PROGRESS,
			Artifact:    base,
			FirstActive: uint64(utils.GetTime().Now().UnixNano() / 1000),
		},
	}

	// Duplicate our log writer for each query.
	result.logger = &serverLogger{
		query_context: result,
		config_obj:    self.config_obj,
		writer:        self.log_writer.Copy(),
	}

	// Keep track of all queries
	self.query_contexts = append(self.query_contexts, result)

	return result
}

func (self *contextManager) GetContext() *flows_proto.ArtifactCollectorContext {
	self.mu.Lock()
	record := proto.Clone(self.context).(*flows_proto.ArtifactCollectorContext)
	self.mu.Unlock()

	// Add each query's status to the context
	record.QueryStats = nil
	for _, query_ctx := range self.query_contexts {
		record.QueryStats = append(record.QueryStats, query_ctx.GetStatus())
	}
	launcher.UpdateFlowStats(record)

	return record
}

// Starts a go routine which saves the context state so the GUI can
// monitor progress.
func (self *contextManager) StartRefresh(wg *sync.WaitGroup) {
	wg.Add(1)
	go func() {
		defer wg.Done()

		for {
			select {
			case <-self.ctx.Done():
				self.Save()
				return

			case <-time.After(time.Duration(10) * time.Second):
				self.Save()
			}
		}
	}()
}

func (self *contextManager) Load() error {
	self.mu.Lock()
	defer self.mu.Unlock()

	launcher, err := services.GetLauncher(self.config_obj)
	if err != nil {
		return err
	}

	details, err := launcher.GetFlowDetails(
		self.config_obj, self.context.ClientId, self.context.SessionId)
	if err != nil {
		return err
	}

	if details.Context == nil {
		return errors.New("Flow context not found")
	}

	self.context = details.Context
	return nil
}

// Flush the context to disk.
func (self *contextManager) Save() error {
	context := self.GetContext()

	self.mu.Lock()
	defer self.mu.Unlock()

	launcher, err := services.GetLauncher(self.config_obj)
	if err != nil {
		return err
	}

	return launcher.WriteFlow(
		self.ctx, self.config_obj, context)
}

func (self *contextManager) Cancel(ctx context.Context, principal string) {
	self.mu.Lock()
	for _, query_ctx := range self.query_contexts {
		query_ctx.UpdateStatus(func(s *crypto_proto.VeloStatus) {
			s.Status = crypto_proto.VeloStatus_GENERIC_ERROR
			s.ErrorMessage = fmt.Sprintf("Cancelled by %v", principal)
		})
	}
	self.mu.Unlock()

	self.cancel()

	self.wg.Wait()
	self.maybeSendCompletionMessage(ctx)
}

func (self *contextManager) Close(ctx context.Context) {
	self.wg.Wait()
	self.maybeSendCompletionMessage(ctx)
}

// Called when each query is completed. Will send the message once for
// the entire flow completion.
func (self *contextManager) maybeSendCompletionMessage(ctx context.Context) {
	flow_context := self.GetContext()
	if flow_context.State == flows_proto.ArtifactCollectorContext_RUNNING {
		return
	}

	row := ordereddict.NewDict().
		Set("Timestamp", utils.GetTime().Now().UTC().Unix()).
		Set("Flow", flow_context).
		Set("FlowId", self.session_id).
		Set("ClientId", "server")

	journal, err := services.GetJournal(self.config_obj)
	if err != nil {
		return
	}
	journal.PushRowsToArtifact(ctx, self.config_obj,
		[]*ordereddict.Dict{row},
		"System.Flow.Completion", "server", self.session_id,
	)
}

func (self *contextManager) RunQuery(
	arg *actions_proto.VQLCollectorArgs) (err error) {

	names_with_response := make(map[string]bool)

	query_context := self.GetQueryContext(arg)
	defer query_context.Close()

	sub_ctx, cancel := context.WithCancel(self.ctx)
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

	flow_path_manager := paths.NewFlowPathManager("server", self.session_id)
	scope := manager.BuildScope(services.ScopeBuilder{
		Config: self.config_obj,

		// For server artifacts, upload() ends up writing in the file
		// store. NOTE: This allows arbitrary filestore write. Using
		// this we can manage the files in the filestore using VQL
		// artifacts.
		Uploader: NewServerUploader(self.config_obj, self.session_id,
			flow_path_manager, self, query_context),

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
			self.config_obj, "server", self.session_id, name, paths.MODE_SERVER)
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
				msg := fmt.Sprintf("ERROR:Query timed out after %v seconds",
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
				self.ChargeRow()
			}
		}
	}

	return nil
}

func (self *contextManager) Logger() LogWriter {
	self.mu.Lock()
	defer self.mu.Unlock()

	return &serverLogger{
		config_obj: self.config_obj,
		writer:     self.log_writer.Copy(),
	}
}
