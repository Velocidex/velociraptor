package hunt_dispatcher

import (
	"context"

	"github.com/Velocidex/ordereddict"
	"google.golang.org/protobuf/encoding/protojson"
	api_proto "www.velocidex.com/golang/velociraptor/api/proto"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	"www.velocidex.com/golang/velociraptor/json"
	"www.velocidex.com/golang/velociraptor/services"
)

// Listenning queue: Server.Internal.HuntUpdate
// Fields:
//   - Hunt - the new version of the hunt. Use this to update local storage.
//   - TriggerParticipation: If set we trigger participation to all
//     directly connected clients.
func (self *HuntDispatcher) ProcessUpdate(
	ctx context.Context,
	config_obj *config_proto.Config,
	row *ordereddict.Dict) error {

	if !self.I_am_master {
		json.Dump(row)
	}

	hunt_any, pres := row.Get("Hunt")
	if !pres {
		return nil
	}

	serialized, err := json.Marshal(hunt_any)
	if err != nil {
		return err
	}

	hunt_obj := &api_proto.Hunt{}
	err = protojson.Unmarshal(serialized, hunt_obj)
	if err != nil {
		return err
	}

	if hunt_obj.State == api_proto.Hunt_DELETED {
		return self.Store.DeleteHunt(ctx, hunt_obj.HuntId)
	}

	// A hunt went into the running state - we need to participate all
	// our currently connected clients.
	_, pres = row.Get("TriggerParticipation")
	if pres {
		err := self.participateAllConnectedClients(
			ctx, config_obj, hunt_obj.HuntId)
		if err != nil {
			return err
		}
	}

	// Only update the version if it is ahead.
	action := self.Store.ModifyHuntObject(ctx, hunt_obj.HuntId,
		func(existing_hunt *HuntRecord) services.HuntModificationAction {
			if existing_hunt.Version < hunt_obj.Version {
				existing_hunt.Hunt = hunt_obj
				return services.HuntPropagateChanges
			}
			return services.HuntUnmodified
		})

	// On the master we also write it to storage.
	if self.I_am_master && action != services.HuntUnmodified {
		err = self.Store.SetHunt(ctx, hunt_obj)
		if err != nil {
			return err
		}
	}

	return nil
}
