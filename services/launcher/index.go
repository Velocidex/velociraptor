package launcher

import (
	"context"
	"sync"
	"time"

	"github.com/Velocidex/ordereddict"
	"github.com/go-errors/errors"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	"www.velocidex.com/golang/velociraptor/constants"
	"www.velocidex.com/golang/velociraptor/datastore"
	"www.velocidex.com/golang/velociraptor/file_store"
	flows_proto "www.velocidex.com/golang/velociraptor/flows/proto"
	"www.velocidex.com/golang/velociraptor/json"
	"www.velocidex.com/golang/velociraptor/paths"
	"www.velocidex.com/golang/velociraptor/result_sets"
	"www.velocidex.com/golang/velociraptor/services"
	"www.velocidex.com/golang/velociraptor/utils"
)

type flowIndexBuilder struct {
	mu        sync.Mutex
	client_id string
	last_time time.Time
}

// Write the flow to the flow resultset index - this is only used for
// the GUI.
func (self *flowIndexBuilder) WriteFlowIndex(
	ctx context.Context,
	config_obj *config_proto.Config,
	flow *flows_proto.ArtifactCollectorContext) error {

	self.mu.Lock()
	defer self.mu.Unlock()

	if flow.Request == nil || flow.SessionId == "" {
		return errors.New("Invalid flow")
	}

	client_path_manager := paths.NewClientPathManager(flow.ClientId)
	journal, err := services.GetJournal(config_obj)
	if err != nil {
		return err
	}

	return journal.AppendToResultSet(config_obj, client_path_manager.FlowIndex(),
		[]*ordereddict.Dict{ordereddict.NewDict().
			Set("FlowId", flow.SessionId).
			Set("Artifacts", flow.Request.Artifacts).
			Set("Created", flow.CreateTime).
			Set("Creator", flow.Request.Creator)},
		services.JournalOptions{
			Sync: true,
		})
}

// Rebuild the flow index from individual flow context files. This can
// be very slow on slow filesystems as it does a lot of IO.
func (self *flowIndexBuilder) BuildFlowIndexFromDatastore(
	ctx context.Context,
	config_obj *config_proto.Config,
	storage_manager services.FlowStorer) error {

	self.mu.Lock()
	defer self.mu.Unlock()

	// Use a non-interruptible ctx to ensure we finish building the
	// index in a reasonable time. Otherwise if the calle cancels the
	// context quickly we might end up with a partial index.
	sub_ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()

	return self.buildFlowIndexFromDatastore(sub_ctx, config_obj, storage_manager)
}

func (self *flowIndexBuilder) buildFlowIndexFromDatastore(
	ctx context.Context,
	config_obj *config_proto.Config,
	storage_manager services.FlowStorer) error {

	// Ignore rebuilds more frequent than a second - this is probably
	// fresh enough.
	if utils.GetTime().Now().Add(-time.Second).Before(self.last_time) {
		return nil
	}

	defer func() {
		self.last_time = utils.GetTime().Now()
	}()

	db, err := datastore.GetDB(config_obj)
	if err != nil {
		return err
	}

	flow_path_manager := paths.NewFlowPathManager(self.client_id, "")
	all_flow_urns, err := db.ListChildren(
		config_obj, flow_path_manager.ContainerPath())
	if err != nil {
		return err
	}

	seen := make(map[string]bool)

	// We only care about the flow contexts
	for _, urn := range all_flow_urns {
		flow_id := urn.Base()
		// Hide the monitoring flow since it is not a real flow.
		if flow_id == constants.MONITORING_WELL_KNOWN_FLOW {
			continue
		}

		seen[flow_id] = true
	}

	flow_reader := NewFlowReader(
		ctx, config_obj, storage_manager, self.client_id)

	go func() {
		defer flow_reader.Close()

		for k := range seen {
			select {
			case <-ctx.Done():
				return
			case flow_reader.In <- k:
			}
		}
	}()

	client_path_manager := paths.NewClientPathManager(self.client_id)
	file_store_factory := file_store.GetFileStore(config_obj)

	rs_writer, err := result_sets.NewResultSetWriter(file_store_factory,
		client_path_manager.FlowIndex(),
		json.DefaultEncOpts(), utils.SyncCompleter, result_sets.TruncateMode)
	if err != nil {
		return err
	}
	defer rs_writer.Close()

	for {
		select {
		case <-ctx.Done():
			return nil

		case flow, ok := <-flow_reader.Out:
			if !ok {
				return nil
			}

			if flow == nil || flow.Request == nil {
				continue
			}

			rs_writer.Write(ordereddict.NewDict().
				Set("FlowId", flow.SessionId).
				Set("Artifacts", flow.Request.Artifacts).
				Set("Created", flow.CreateTime).
				Set("Creator", flow.Request.Creator))
		}
	}
}

// Filter the index through the journal.
func (self *flowIndexBuilder) RemoveClientFlowsFromIndex(
	ctx context.Context, config_obj *config_proto.Config,
	storage_manager services.FlowStorer,
	flows map[string]bool) error {

	self.mu.Lock()
	defer self.mu.Unlock()

	client_path_manager := paths.NewClientPathManager(self.client_id)
	file_store_factory := file_store.GetFileStore(config_obj)
	rs_reader, err := result_sets.NewResultSetReader(file_store_factory,
		client_path_manager.FlowIndex())
	if err != nil {
		// No existing result set, build from scratch.
		return self.buildFlowIndexFromDatastore(ctx, config_obj, storage_manager)
	}
	defer rs_reader.Close()

	rs_writer, err := result_sets.NewResultSetWriter(file_store_factory,
		client_path_manager.FlowIndex(),
		json.DefaultEncOpts(), utils.SyncCompleter, result_sets.TruncateMode)
	if err != nil {
		return err
	}
	defer rs_writer.Close()

	for r := range rs_reader.Rows(ctx) {
		flow_id, _ := r.GetString("FlowId")
		_, ok := flows[flow_id]
		if ok {
			continue
		}
		rs_writer.Write(r)
	}

	return nil
}
