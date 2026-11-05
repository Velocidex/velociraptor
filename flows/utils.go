package flows

import (
	"context"
	"time"

	"github.com/Velocidex/ordereddict"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	flows_proto "www.velocidex.com/golang/velociraptor/flows/proto"
	artifact_paths "www.velocidex.com/golang/velociraptor/paths/artifacts"
	"www.velocidex.com/golang/velociraptor/services"
)

func SendFlowCompletion(
	ctx context.Context,
	config_obj *config_proto.Config,
	flow *flows_proto.ArtifactCollectorContext) error {

	row := ordereddict.NewDict().
		Set("Timestamp", time.Now().UTC().Unix()).
		Set("Flow", flow).
		Set("FlowId", flow.SessionId).
		Set("ClientId", flow.ClientId)

	client_id := flow.ClientId

	journal, err := services.GetJournal(config_obj)
	if err != nil {
		return err
	}

	journal.PushRowsToArtifactAsync(ctx,
		config_obj, row, artifact_paths.FLOW_COMPLETION.
			WithSuperUser().WithFrom(client_id).
			WithClientId(client_id))
	return nil
}
