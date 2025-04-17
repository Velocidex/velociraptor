package launcher

import (
	"context"
	"fmt"
	"io"
	"os"
	"sync"
	"time"

	"github.com/Velocidex/ordereddict"
	"github.com/go-errors/errors"
	api_proto "www.velocidex.com/golang/velociraptor/api/proto"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	crypto_proto "www.velocidex.com/golang/velociraptor/crypto/proto"
	"www.velocidex.com/golang/velociraptor/datastore"
	"www.velocidex.com/golang/velociraptor/file_store"
	flows_proto "www.velocidex.com/golang/velociraptor/flows/proto"
	"www.velocidex.com/golang/velociraptor/json"
	"www.velocidex.com/golang/velociraptor/logging"
	"www.velocidex.com/golang/velociraptor/paths"
	"www.velocidex.com/golang/velociraptor/result_sets"
	"www.velocidex.com/golang/velociraptor/services"
	"www.velocidex.com/golang/velociraptor/utils"
)

type FlowStorageManager struct {
	mu sync.Mutex

	pendingIndexes []string
	indexBuilders  map[string]*flowIndexBuilder

	flow_journal_mu sync.Mutex
}

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
		err = self.buildFlowIndexFromDatastore(ctx, config_obj, client_id)
		if err != nil {
			return nil, 0, fmt.Errorf("buildFlowIndexFromDatastore %w", err)
		}

		rs_reader, err = result_sets.NewResultSetReaderWithOptions(
			ctx, config_obj, file_store_factory,
			client_path_manager.FlowIndex(), options)
		if err != nil {
			return nil, 0, fmt.Errorf("ListFlows %w", err)
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

// Rebuild flow indexes periodically as required.
func (self *FlowStorageManager) houseKeeping(
	ctx context.Context,
	config_obj *config_proto.Config,
	wg *sync.WaitGroup) {

	defer wg.Done()

	delay := time.Second * 60
	if config_obj.Defaults != nil &&
		config_obj.Defaults.ClientInfoHousekeepingPeriod > 0 {
		delay = time.Duration(config_obj.Defaults.ClientInfoHousekeepingPeriod) * time.Second
	}

	logger := logging.GetLogger(config_obj, &logging.FrontendComponent)

	for {
		select {
		case <-ctx.Done():
			return

		case <-utils.GetTime().After(utils.Jitter(delay)):
			logger.Debug("<green>FlowStorageManager</> housekeeping run")
			err := self.removeFlowsFromJournal(ctx, config_obj)
			if err != nil {
				logger.Error("removeFlowsFromJournal: %v", err)
			}
		}
	}
}

func (self *FlowStorageManager) buildFlowIndexFromDatastore(
	ctx context.Context,
	config_obj *config_proto.Config,
	client_id string) error {

	// Do not hold the lock while we build different clients.
	self.mu.Lock()
	builder, pres := self.indexBuilders[client_id]
	if !pres {
		builder = &flowIndexBuilder{
			client_id: client_id,
		}
	}
	self.mu.Unlock()

	err := builder.buildFlowIndexFromDatastore(ctx, config_obj, self, client_id)
	self.mu.Lock()
	delete(self.indexBuilders, client_id)
	self.mu.Unlock()

	return err
}

// Load the collector context from storage.
func (self *FlowStorageManager) LoadCollectionContext(
	ctx context.Context,
	config_obj *config_proto.Config,
	client_id, flow_id string) (*flows_proto.ArtifactCollectorContext, error) {

	in_flight_time := int64(0)
	client_info_manager, err := services.GetClientInfoManager(config_obj)
	if err != nil {
		return nil, err
	}

	err = client_info_manager.Modify(ctx, client_id,
		func(client_info *services.ClientInfo) (*services.ClientInfo, error) {
			if client_info != nil &&
				client_info.InFlightFlows != nil {
				in_flight_time, _ = client_info.InFlightFlows[flow_id]
			}
			return nil, nil
		})
	if err != nil {
		return nil, err
	}

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

	collection_context.InflightTime = uint64(in_flight_time)

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

func NewFlowStorageManager(
	ctx context.Context,
	config_obj *config_proto.Config,
	wg *sync.WaitGroup) *FlowStorageManager {
	res := &FlowStorageManager{}
	wg.Add(1)
	go res.houseKeeping(ctx, config_obj, wg)

	return res
}
