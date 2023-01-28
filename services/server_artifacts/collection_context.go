package server_artifacts

import (
	"context"
	"sync"
	"time"

	"github.com/go-errors/errors"
	"google.golang.org/protobuf/proto"
	"www.velocidex.com/golang/velociraptor/actions"
	actions_proto "www.velocidex.com/golang/velociraptor/actions/proto"
	"www.velocidex.com/golang/velociraptor/artifacts"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	crypto_proto "www.velocidex.com/golang/velociraptor/crypto/proto"
	"www.velocidex.com/golang/velociraptor/datastore"
	flows_proto "www.velocidex.com/golang/velociraptor/flows/proto"
	"www.velocidex.com/golang/velociraptor/paths"
	"www.velocidex.com/golang/velociraptor/services"
	"www.velocidex.com/golang/velociraptor/services/launcher"
	"www.velocidex.com/golang/velociraptor/utils"
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
}

func NewCollectionContextManager(
	ctx context.Context,
	config_obj *config_proto.Config,
	client_id string,
	flow_id string) (CollectionContextManager, error) {

	sub_ctx, cancel := context.WithCancel(ctx)
	log_writer, err := NewServerLogWriter(sub_ctx, config_obj, flow_id)
	if err != nil {
		return nil, err
	}

	self := &contextManager{
		config_obj: config_obj,
		context: &flows_proto.ArtifactCollectorContext{
			ClientId:  client_id,
			SessionId: flow_id,
		},
		ctx:        sub_ctx,
		cancel:     cancel,
		wg:         &sync.WaitGroup{},
		session_id: flow_id,
		log_writer: &counterWriter{ResultSetWriter: log_writer},
	}

	// Write the collection context periodically to disk so the GUI
	// can track progress.
	self.StartRefresh()

	return self, self.Load(self.context)
}

// Prepare a new query context for this request.
func (self *contextManager) GetQueryContext(
	query *actions_proto.VQLCollectorArgs) QueryContext {

	self.mu.Lock()
	defer self.mu.Unlock()

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
			Status: crypto_proto.VeloStatus_PROGRESS,
			Artifact: artifacts.DeobfuscateString(
				self.config_obj, actions.GetQueryName(query.Query)),
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
func (self *contextManager) StartRefresh() {
	self.wg.Add(1)
	go func() {
		defer self.wg.Done()

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

func (self *contextManager) Load(
	context *flows_proto.ArtifactCollectorContext) error {
	self.mu.Lock()
	defer self.mu.Unlock()

	launcher, err := services.GetLauncher(self.config_obj)
	if err != nil {
		return err
	}

	details, err := launcher.GetFlowDetails(
		self.config_obj, context.ClientId, context.SessionId)
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

	db, err := datastore.GetDB(self.config_obj)
	if err != nil {
		return err
	}

	flow_path_manager := paths.NewFlowPathManager("server", self.session_id)
	return db.SetSubjectWithCompletion(
		self.config_obj, flow_path_manager.Path(),
		context, utils.BackgroundWriter)
}

func (self *contextManager) Cancel() {
	self.cancel()
	self.wg.Wait()
}

func (self *contextManager) Logger() LogWriter {
	self.mu.Lock()
	defer self.mu.Unlock()

	return &serverLogger{
		config_obj: self.config_obj,
		writer:     self.log_writer.Copy(),
	}
}
