package launcher

import (
	"context"

	api_proto "www.velocidex.com/golang/velociraptor/api/proto"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	crypto_proto "www.velocidex.com/golang/velociraptor/crypto/proto"
	flows_proto "www.velocidex.com/golang/velociraptor/flows/proto"
	"www.velocidex.com/golang/velociraptor/result_sets"
	"www.velocidex.com/golang/velociraptor/services"
	"www.velocidex.com/golang/velociraptor/utils"
)

var (
	notAvailableError = utils.Wrap(utils.InvalidConfigError,
		"FlowStorer not available without a filestore")
)

type DummyStorer struct{}

func (self *DummyStorer) WriteFlow(
	ctx context.Context,
	config_obj *config_proto.Config,
	flow *flows_proto.ArtifactCollectorContext,
	completion func()) error {
	return notAvailableError
}

func (self *DummyStorer) WriteFlowStats(
	ctx context.Context,
	config_obj *config_proto.Config,
	flow *flows_proto.ArtifactCollectorContext,
	completion func()) error {
	return notAvailableError
}

func (self *DummyStorer) WriteFlowIndex(
	ctx context.Context,
	config_obj *config_proto.Config,
	flow *flows_proto.ArtifactCollectorContext) error {
	return notAvailableError
}

func (self *DummyStorer) WriteTask(
	ctx context.Context,
	config_obj *config_proto.Config,
	client_id string,
	msg *crypto_proto.VeloMessage) error {
	return notAvailableError
}

func (self *DummyStorer) DeleteFlow(
	ctx context.Context,
	config_obj *config_proto.Config,
	client_id string, flow_id string, principal string,
	options services.DeleteFlowOptions) ([]*services.DeleteFlowResponse, error) {
	return nil, notAvailableError
}

func (self *DummyStorer) LoadCollectionContext(
	ctx context.Context,
	config_obj *config_proto.Config,
	client_id, flow_id string) (*flows_proto.ArtifactCollectorContext, error) {
	return nil, notAvailableError
}

func (self *DummyStorer) ListFlows(
	ctx context.Context,
	config_obj *config_proto.Config,
	client_id string,
	options result_sets.ResultSetOptions,
	offset, length int64) ([]*services.FlowSummary, int64, error) {
	return nil, 0, notAvailableError
}

// Get the exact requests that were sent for this collection (for
// provenance).
func (self *DummyStorer) GetFlowRequests(
	ctx context.Context,
	config_obj *config_proto.Config,
	client_id string, flow_id string,
	offset uint64, count uint64) (*api_proto.ApiFlowRequestDetails, error) {
	return nil, notAvailableError
}
