package hunt_dispatcher

import (
	"context"

	"github.com/Velocidex/ordereddict"
	api_proto "www.velocidex.com/golang/velociraptor/api/proto"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	"www.velocidex.com/golang/velociraptor/services"
)

// This is called by the local server to mutate the hunt
// object. Mutations include increasing the number of clients
// assigned, completed etc. These mutations may happen very frequently
// and so we do not want to flush them to disk immediately. Instead we
// push the mutations to the master node's hunt manager, where they
// will be applied on the master node. Eventually these will end up in
// the filesystem and possibly refreshed into this dispatcher.
// Therefore, writers may write mutations and expect they take an
// unspecified time to appear in the hunt details.
func (self *HuntDispatcher) MutateHunt(
	ctx context.Context, config_obj *config_proto.Config,
	mutation *api_proto.HuntMutation) error {
	journal, err := services.GetJournal(config_obj)
	if err != nil {
		return err
	}

	journal.PushRowsToArtifactAsync(ctx, config_obj,
		ordereddict.NewDict().
			Set("hunt_id", mutation.HuntId).
			Set("mutation", mutation),
		"Server.Internal.HuntModification")
	return nil
}
