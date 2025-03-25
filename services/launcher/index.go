package launcher

import (
	"context"
	"sync"
	"time"

	"github.com/Velocidex/ordereddict"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	"www.velocidex.com/golang/velociraptor/constants"
	"www.velocidex.com/golang/velociraptor/datastore"
	"www.velocidex.com/golang/velociraptor/file_store"
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

// Rebuild the flow index from individual flow context files. This can
// be very slow on slow filesystems as it does a lot of IO.
func (self *flowIndexBuilder) buildFlowIndexFromDatastore(
	ctx context.Context,
	config_obj *config_proto.Config,
	storage_manager services.FlowStorer,
	client_id string) error {

	self.mu.Lock()
	defer self.mu.Unlock()

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

	flow_path_manager := paths.NewFlowPathManager(client_id, "")
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
		ctx, config_obj, storage_manager, client_id)

	go func() {
		defer flow_reader.Close()

		for k := range seen {
			flow_reader.In <- k
		}
	}()

	client_path_manager := paths.NewClientPathManager(client_id)
	file_store_factory := file_store.GetFileStore(config_obj)

	rs_writer, err := result_sets.NewResultSetWriter(file_store_factory,
		client_path_manager.FlowIndex(),
		json.DefaultEncOpts(), utils.SyncCompleter, result_sets.TruncateMode)
	if err != nil {
		return err
	}
	defer rs_writer.Close()

	for flow := range flow_reader.Out {
		rs_writer.Write(ordereddict.NewDict().
			Set("FlowId", flow.SessionId).
			Set("Artifacts", flow.Request.Artifacts).
			Set("Created", flow.CreateTime).
			Set("Creator", flow.Request.Creator))
	}

	return nil
}
