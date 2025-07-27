package launcher

import (
	"context"
	"fmt"
	"io"
	"os"
	"sync"
	"time"

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

	indexBuilders map[string]*flowIndexBuilder

	// Protects the global flows journal
	flow_journal_mu sync.Mutex

	// Throttle index rebuilds so they are not too frequent.
	throttler          *utils.Throttler
	concurrencyControl *utils.Concurrency
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

func (self *FlowStorageManager) WriteFlowStats(
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
		config_obj, flow_path_manager.Stats(), flow, completion)
}

// Write the flow to the flow resultset index - this is only used for
// the GUI.
func (self *FlowStorageManager) WriteFlowIndex(
	ctx context.Context,
	config_obj *config_proto.Config,
	flow *flows_proto.ArtifactCollectorContext) error {

	return self.GetIndexBuilder(flow.ClientId).WriteFlowIndex(
		ctx, config_obj, flow)
}

func (self *FlowStorageManager) RemoveClientFlowsFromIndex(
	ctx context.Context, config_obj *config_proto.Config,
	client_id string, flows map[string]bool) error {

	return self.GetIndexBuilder(client_id).
		RemoveClientFlowsFromIndex(ctx, config_obj, self, flows)
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

func (self *FlowStorageManager) shouldRefreshRS(
	config_obj *config_proto.Config,
	rs_reader result_sets.ResultSetReader) bool {

	if rs_reader.TotalRows() <= 0 {
		return true
	}

	now := utils.GetTime().Now()
	max_age := 600 * time.Second
	if config_obj.Defaults != nil &&
		config_obj.Defaults.ReindexPeriodSeconds > 0 {
		max_age = time.Duration(config_obj.Defaults.ReindexPeriodSeconds) * time.Second
	}

	// The reader is not older than max_age, lets just use it.
	if now.Add(-max_age).Before(rs_reader.MTime()) {
		return false
	}

	// Only reindex if we are ready - this helps to spread out the
	// load when we read flow indexes very quickly (e.g in a VQL
	// query)
	return self.throttler.Ready()
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

	if err != nil || self.shouldRefreshRS(config_obj, rs_reader) {
		// Try to get concurrency here - if we fail, we just make do
		// with the old result set - no big deal.
		closer, err := self.concurrencyControl.StartConcurrencyControl(ctx)
		if err == nil {
			defer closer()

			// Try to rebuild the index
			err = self.buildFlowIndexFromDatastore(
				ctx, config_obj, client_id)
			if err != nil {
				return nil, 0, fmt.Errorf("buildFlowIndexFromDatastore %w", err)
			}
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
		last_try := utils.GetTime().Now()

		select {
		case <-ctx.Done():
			return

		case <-utils.GetTime().After(utils.Jitter(delay)):
			// Avoid retrying too quickly. This is mainly for
			// tests where the time is mocked for the After(delay)
			// above does not work.
			if utils.GetTime().Now().Sub(last_try) < time.Second*10 {
				utils.SleepWithCtx(ctx, time.Minute)
				continue
			}

			logger.Debug("<green>FlowStorageManager</> housekeeping run")
			err := self.RemoveFlowsFromJournal(ctx, config_obj)
			if err != nil {
				logger.Error("RemoveFlowsFromJournal: %v", err)
			}
		}
	}
}

func (self *FlowStorageManager) GetIndexBuilder(client_id string) *flowIndexBuilder {
	self.mu.Lock()
	defer self.mu.Unlock()

	builder, pres := self.indexBuilders[client_id]
	if !pres {
		builder = &flowIndexBuilder{
			client_id: client_id,
		}
		self.indexBuilders[client_id] = builder
	}

	return builder
}

func (self *FlowStorageManager) buildFlowIndexFromDatastore(
	ctx context.Context,
	config_obj *config_proto.Config,
	client_id string) error {

	// Do not hold the lock while we build different clients.
	return self.GetIndexBuilder(client_id).
		BuildFlowIndexFromDatastore(ctx, config_obj, self)
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

	collection_context.TransactionsOutstanding = stats_context.TransactionsOutstanding
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
	res := &FlowStorageManager{
		indexBuilders: make(map[string]*flowIndexBuilder),
		throttler:     utils.NewThrottlerWithDuration(time.Second),

		// Do not allow more than one reindex at the same time. If we
		// cant get to reindex quickly, we just dont worry about it
		// and use the old index snapshot.
		concurrencyControl: utils.NewConcurrencyControl(
			1, 100*time.Millisecond),
	}

	wg.Add(1)
	go res.houseKeeping(ctx, config_obj, wg)

	return res
}
