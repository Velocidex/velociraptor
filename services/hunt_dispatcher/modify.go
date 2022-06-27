package hunt_dispatcher

import (
	"context"
	"time"

	"github.com/Velocidex/ordereddict"
	api_proto "www.velocidex.com/golang/velociraptor/api/proto"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	"www.velocidex.com/golang/velociraptor/services"
)

// This method modifies the hunt. Only the following modifications are allowed:

// 1. A hunt in the paused state can go to the running state. This
//    will update the StartTime.
// 2. A hunt in the running state can go to the Stop state
// 3. A hunt's description can be modified.
func (self *HuntDispatcher) ModifyHunt(
	ctx context.Context,
	config_obj *config_proto.Config,
	hunt_modification *api_proto.Hunt,
	user string) error {

	// We can not modify the hunt directly, instead we send a
	// mutation to the hunt manager on the master.
	mutation := &api_proto.HuntMutation{
		HuntId:      hunt_modification.HuntId,
		Description: hunt_modification.HuntDescription,
	}

	// Is the description changed?
	if hunt_modification.HuntDescription != "" {
		mutation.Description = hunt_modification.HuntDescription

		// Archive the hunt.
	} else if hunt_modification.State == api_proto.Hunt_ARCHIVED {
		mutation.State = api_proto.Hunt_ARCHIVED

		row := ordereddict.NewDict().
			Set("Timestamp", time.Now().UTC().Unix()).
			Set("HuntId", mutation.HuntId).
			Set("User", user)

		// Alert listeners that the hunt is being archived.
		journal, err := services.GetJournal(config_obj)
		if err != nil {
			return err
		}

		err = journal.PushRowsToArtifact(config_obj,
			[]*ordereddict.Dict{row}, "System.Hunt.Archive",
			"server", hunt_modification.HuntId)
		if err != nil {
			return err
		}

		// We are trying to start or restart the hunt.
	} else if hunt_modification.State == api_proto.Hunt_RUNNING {

		// We allow restarting stopped hunts
		// but this may not work as intended
		// because we still have a hunt index
		// - i.e. clients that already
		// scheduled the hunt will not
		// re-schedule (whether they ran it or
		// not). Usually the most reliable way
		// to re-do a hunt is to copy it and
		// do it again.
		mutation.State = api_proto.Hunt_RUNNING
		mutation.StartTime = uint64(time.Now().UnixNano() / 1000)

		// We are trying to pause or stop the hunt.
	} else if hunt_modification.State == api_proto.Hunt_STOPPED ||
		hunt_modification.State == api_proto.Hunt_PAUSED {
		mutation.State = api_proto.Hunt_STOPPED
	}

	return self.MutateHunt(config_obj, mutation)
}
