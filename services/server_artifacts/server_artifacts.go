/*

   This service provides a runner to execute artifacts on the server.
   Server artifacts run with high privilege and so only users with the
   COLLECT_SERVER permission can run those, unless the artifact is
   marked as BASIC by the artifact metadata.

   Server artifacts are used for administration or information purposes.

*/

package server_artifacts

import (
	"context"
	"sync"

	"github.com/Velocidex/ordereddict"
	"www.velocidex.com/golang/velociraptor/acls"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	crypto_proto "www.velocidex.com/golang/velociraptor/crypto/proto"
	flows_proto "www.velocidex.com/golang/velociraptor/flows/proto"
	"www.velocidex.com/golang/velociraptor/json"
	"www.velocidex.com/golang/velociraptor/logging"
	"www.velocidex.com/golang/velociraptor/paths/artifacts"
	"www.velocidex.com/golang/velociraptor/services"
	"www.velocidex.com/golang/velociraptor/utils"
)

// The Server Artifact Service is responsible for running server side
// VQL artifacts.

// Currently there is only a single server artifact runner (for each
// org), running on the master node.

type ServerArtifactRunner struct {
	ctx        context.Context
	config_obj *config_proto.Config
	wg         *sync.WaitGroup
}

// Create a bare ServerArtifactsService without the extra management.
func NewServerArtifactRunner(
	ctx context.Context,
	config_obj *config_proto.Config, wg *sync.WaitGroup) (
	*ServerArtifactRunner, error) {
	res := &ServerArtifactRunner{
		ctx:        ctx,
		config_obj: config_obj,
		wg:         wg,
	}

	return res, res.Start(ctx, config_obj, wg)
}

func (self *ServerArtifactRunner) LaunchServerArtifact(
	config_obj *config_proto.Config,
	session_id string,
	req *crypto_proto.FlowRequest,
	collection_context *flows_proto.ArtifactCollectorContext) error {

	// Build a request
	scheduler_req := &ServerArtifactRequest{
		Req:               req,
		CollectionContext: collection_context,
	}

	scheduler, err := services.GetSchedulerService(config_obj)
	if err != nil {
		return err
	}

	result_chan, err := scheduler.Schedule(self.ctx, services.SchedulerJob{
		Queue: "ServerArtifact",
		Job:   json.MustMarshalString(scheduler_req),
		OrgId: config_obj.OrgId,
		Done:  func(result string, err error) {},
	})
	if err != nil {
		return err
	}

	// We don't actually care about the response from the job anymore.
	services.DrainAndDiscardResponses(result_chan)

	return nil
}

func (self *ServerArtifactRunner) Cancel(
	ctx context.Context, flow_id, principal string) error {

	ok, err := services.CheckAccess(self.config_obj, principal, acls.COLLECT_SERVER)
	if err != nil || !ok {
		return utils.Wrap(utils.PermissionDenied, "Cancel server collection %v", flow_id)
	}

	err = CancelFlow(ctx, self.config_obj, flow_id, principal)
	if err != nil {
		return err
	}

	// Let the minion know to cancel this collection. The minion may
	// be offline which will make this message lost, but it is ok -
	// the flow state is correct now.
	journal, err := services.GetJournal(self.config_obj)
	if err != nil {
		return err
	}

	journal.PushRowsToArtifactAsync(
		ctx, self.config_obj, ordereddict.NewDict().
			Set("ClientId", "server").
			Set("FlowId", flow_id).
			Set("Principal", principal),
		artifacts.CANCEL_TASK.
			WithFrom(principal).
			WithSuperUser())

	return nil
}

func NewServerArtifactService(
	ctx context.Context,
	wg *sync.WaitGroup,
	config_obj *config_proto.Config) (services.ServerArtifactRunner, error) {

	self, err := NewServerArtifactRunner(ctx, config_obj, wg)
	if err != nil {
		return nil, err
	}

	logger := logging.GetLogger(
		config_obj, &logging.FrontendComponent)
	logger.Info("<green>Starting</> Server Artifact Runner Service for %v",
		services.GetOrgName(config_obj))

	return self, nil
}
