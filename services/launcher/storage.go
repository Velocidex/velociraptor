package launcher

import (
	"context"
	"fmt"
	"io"
	"os"

	"github.com/Velocidex/ordereddict"
	"github.com/go-errors/errors"
	api_proto "www.velocidex.com/golang/velociraptor/api/proto"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	"www.velocidex.com/golang/velociraptor/constants"
	crypto_proto "www.velocidex.com/golang/velociraptor/crypto/proto"
	"www.velocidex.com/golang/velociraptor/datastore"
	"www.velocidex.com/golang/velociraptor/file_store"
	flows_proto "www.velocidex.com/golang/velociraptor/flows/proto"
	"www.velocidex.com/golang/velociraptor/json"
	"www.velocidex.com/golang/velociraptor/paths"
	"www.velocidex.com/golang/velociraptor/result_sets"
	"www.velocidex.com/golang/velociraptor/services"
	"www.velocidex.com/golang/velociraptor/utils"
)

type FlowStorageManager struct{}

func (self *FlowStorageManager) WriteFlow(
	ctx context.Context,
	config_obj *config_proto.Config,
	flow *flows_proto.ArtifactCollectorContext,
	completion func()) error {

	db, err := datastore.GetDB(config_obj)
	if err != nil {
		return err
	}

	flow_path_manager := paths.NewFlowPathManager(flow.ClientId, flow.SessionId)
	return db.SetSubjectWithCompletion(
		config_obj, flow_path_manager.Path(), flow, completion)
}

// Write the flow to the flow resultset index - this is only used for
// the GUI.
func (self *FlowStorageManager) WriteFlowIndex(
	ctx context.Context,
	config_obj *config_proto.Config,
	flow *flows_proto.ArtifactCollectorContext) error {

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

func (self *FlowStorageManager) WriteTask(
	ctx context.Context,
	config_obj *config_proto.Config,
	client_id string, msg *crypto_proto.VeloMessage) error {

	db, err := datastore.GetDB(config_obj)
	if err != nil {
		return err
	}

	flow_path_manager := paths.NewFlowPathManager(client_id, msg.SessionId)
	return db.SetSubjectWithCompletion(
		config_obj, flow_path_manager.Task(),
		&api_proto.ApiFlowRequestDetails{
			Items: []*crypto_proto.VeloMessage{msg},
		}, utils.BackgroundWriter)
}

func (self *FlowStorageManager) ListFlows(
	ctx context.Context,
	config_obj *config_proto.Config,
	client_id string,
	options result_sets.ResultSetOptions,
	offset int64, length int64) ([]*services.FlowSummary, int64, error) {

	client_path_manager := paths.NewClientPathManager(client_id)
	file_store_factory := file_store.GetFileStore(config_obj)
	rs_reader, err := result_sets.NewResultSetReaderWithOptions(
		ctx, config_obj, file_store_factory,
		client_path_manager.FlowIndex(), options)
	if err != nil || rs_reader.TotalRows() <= 0 {
		// Try to rebuild the index
		err = self.buildFlowIndexFromLegacy(ctx, config_obj, client_id)
		if err != nil {
			return nil, 0, fmt.Errorf("buildFlowIndexFromLegacy %w", err)
		}

		rs_reader, err = result_sets.NewResultSetReaderWithOptions(
			ctx, config_obj, file_store_factory,
			client_path_manager.FlowIndex(), options)
		if err != nil {
			return nil, 0, fmt.Errorf("NewResultSetReaderWithOptions %w", err)
		}
	}

	if err != nil {
		return nil, 0, err
	}

	result := []*services.FlowSummary{}
	err = rs_reader.SeekToRow(offset)
	if errors.Is(err, io.EOF) {
		return result, 0, nil
	}

	if err != nil {
		return nil, 0, fmt.Errorf("SeekToRow %v %w", offset, err)
	}

	// Highly optimized reader for speed.
	json_chan, err := rs_reader.JSON(ctx)
	if err != nil {
		return nil, 0, fmt.Errorf("JSON %w", err)
	}

	for serialized := range json_chan {
		summary := &services.FlowSummary{}
		err = json.Unmarshal(serialized, summary)
		if err == nil {
			result = append(result, summary)
		}
		if int64(len(result)) >= length {
			break
		}
	}

	return result, rs_reader.TotalRows(), nil
}

// Rebuild the flow index from individual flow context files.
func (self *FlowStorageManager) buildFlowIndexFromLegacy(
	ctx context.Context,
	config_obj *config_proto.Config,
	client_id string) error {

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
		ctx, config_obj, self, client_id)

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

// Load the collector context from storage.
func (self *FlowStorageManager) LoadCollectionContext(
	ctx context.Context,
	config_obj *config_proto.Config,
	client_id, flow_id string) (*flows_proto.ArtifactCollectorContext, error) {

	flow_path_manager := paths.NewFlowPathManager(client_id, flow_id)
	collection_context := &flows_proto.ArtifactCollectorContext{}
	db, err := datastore.GetDB(config_obj)
	if err != nil {
		return nil, err
	}

	err = db.GetSubject(
		config_obj, flow_path_manager.Path(), collection_context)
	if errors.Is(err, os.ErrNotExist) {
		return nil, fmt.Errorf("%w: %v in client '%v'",
			services.FlowNotFoundError, flow_id, client_id)
	}
	if err != nil {
		return nil, err
	}

	if collection_context.SessionId == "" {
		return nil, fmt.Errorf("%w: %v in client '%v'",
			services.FlowNotFoundError, flow_id, client_id)
	}

	// Try to open the stats context
	stats_context := &flows_proto.ArtifactCollectorContext{}
	err = db.GetSubject(
		config_obj, flow_path_manager.Stats(), stats_context)
	// Stats file is missing that is ok and not an error.
	if err != nil {
		UpdateFlowStats(collection_context)
		return collection_context, nil
	}

	if len(stats_context.QueryStats) > 0 {
		collection_context.QueryStats = stats_context.QueryStats
	}

	UpdateFlowStats(collection_context)
	return collection_context, nil
}

func (self *FlowStorageManager) GetFlowRequests(
	ctx context.Context,
	config_obj *config_proto.Config,
	client_id string, flow_id string,
	offset uint64, count uint64) (*api_proto.ApiFlowRequestDetails, error) {
	if count == 0 {
		count = 50
	}

	db, err := datastore.GetDB(config_obj)
	if err != nil {
		return nil, err
	}

	result := &api_proto.ApiFlowRequestDetails{}

	flow_path_manager := paths.NewFlowPathManager(client_id, flow_id)
	flow_details := &api_proto.ApiFlowRequestDetails{}
	err = db.GetSubject(
		config_obj, flow_path_manager.Task(), flow_details)
	if errors.Is(err, os.ErrNotExist) {
		return nil, fmt.Errorf("%w: %v in client '%v'",
			services.FlowNotFoundError, flow_id, client_id)
	}

	if err != nil {
		return nil, err
	}

	if offset > uint64(len(flow_details.Items)) {
		return result, nil
	}

	end := offset + count
	if end > uint64(len(flow_details.Items)) {
		end = uint64(len(flow_details.Items))
	}

	result.Items = flow_details.Items[offset:end]
	return result, nil
}
