package launcher

import (
	"context"
	"sort"

	"github.com/go-errors/errors"
	api_proto "www.velocidex.com/golang/velociraptor/api/proto"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	"www.velocidex.com/golang/velociraptor/constants"
	crypto_proto "www.velocidex.com/golang/velociraptor/crypto/proto"
	"www.velocidex.com/golang/velociraptor/datastore"
	flows_proto "www.velocidex.com/golang/velociraptor/flows/proto"
	"www.velocidex.com/golang/velociraptor/paths"
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
	client_id string) (result []string, err error) {

	db, err := datastore.GetDB(config_obj)
	if err != nil {
		return nil, err
	}

	flow_path_manager := paths.NewFlowPathManager(client_id, "")
	all_flow_urns, err := db.ListChildren(
		config_obj, flow_path_manager.ContainerPath())
	if err != nil {
		return nil, err
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

	for k := range seen {
		result = append(result, k)
	}

	sort.Strings(result)

	return result, nil
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
	if err != nil {
		return nil, err
	}

	if collection_context.SessionId == "" {
		return nil, errors.New("Unknown flow " + client_id + " " + flow_id)
	}

	// Try to open the stats context
	stats_context := &flows_proto.ArtifactCollectorContext{}
	err = db.GetSubject(
		config_obj, flow_path_manager.Stats(), stats_context)
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
