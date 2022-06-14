package server_artifacts

import (
	"context"
	"sync"
	"time"

	"github.com/pkg/errors"
	"google.golang.org/protobuf/proto"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	"www.velocidex.com/golang/velociraptor/datastore"
	flows_proto "www.velocidex.com/golang/velociraptor/flows/proto"
	"www.velocidex.com/golang/velociraptor/paths"
	"www.velocidex.com/golang/velociraptor/services"
)

type CollectionContextManager interface {
	GetContext() *flows_proto.ArtifactCollectorContext
	Modify(cb func(context *flows_proto.ArtifactCollectorContext))
	Load(context *flows_proto.ArtifactCollectorContext) error
	Save() error
	Close()
}

type contextManager struct {
	context      *flows_proto.ArtifactCollectorContext
	mu           sync.Mutex
	config_obj   *config_proto.Config
	path_manager *paths.FlowPathManager
	cancel       func()
}

func NewCollectionContextManager(
	ctx context.Context,
	config_obj *config_proto.Config,
	client_id string,
	flow_id string) (CollectionContextManager, error) {

	sub_ctx, cancel := context.WithCancel(ctx)
	self := &contextManager{
		config_obj:   config_obj,
		cancel:       cancel,
		path_manager: paths.NewFlowPathManager(client_id, flow_id),
		context: &flows_proto.ArtifactCollectorContext{
			ClientId:  client_id,
			SessionId: flow_id,
		},
	}

	// Write collection context periodically to disk so the
	// GUI can track progress.
	self.StartRefresh(sub_ctx)

	return self, self.Load(self.context)
}

func (self *contextManager) Close() {
	self.cancel()
}

func (self *contextManager) GetContext() *flows_proto.ArtifactCollectorContext {
	self.mu.Lock()
	defer self.mu.Unlock()

	return proto.Clone(self.context).(*flows_proto.ArtifactCollectorContext)
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

	launcher, err := services.GetLauncher()
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
	self.mu.Lock()
	defer self.mu.Unlock()

	// Ignore collections which are not running.
	launcher, err := services.GetLauncher()
	if err != nil {
		return err
	}

	details, err := launcher.GetFlowDetails(
		self.config_obj, self.context.ClientId,
		self.context.SessionId)
	if err == nil && details.Context != nil &&
		details.Context.Request != nil &&
		details.Context.State != flows_proto.ArtifactCollectorContext_RUNNING {
		return nil
	}

	db, err := datastore.GetDB(self.config_obj)
	if err != nil {
		return err
	}
	return db.SetSubjectWithCompletion(
		self.config_obj, self.path_manager.Path(), self.context, nil)
}
