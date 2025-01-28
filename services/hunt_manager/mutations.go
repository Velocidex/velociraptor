package hunt_manager

import (
	"context"
	"errors"
	"sort"

	"github.com/Velocidex/ordereddict"
	api_proto "www.velocidex.com/golang/velociraptor/api/proto"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	"www.velocidex.com/golang/velociraptor/paths"
	"www.velocidex.com/golang/velociraptor/services"
	"www.velocidex.com/golang/velociraptor/utils"
)

// The hunt_manager only runs on the master node. It therefore is the
// only component to interact with hunt data stored on disk.
// This interaction is mediated through mutations.

// Mutations are sent from any node via the
// Server.Internal.HuntModification queue which only the master is
// listening to.

// Modify a hunt object through a mutation.
func (self *HuntManager) ProcessMutation(
	ctx context.Context,
	config_obj *config_proto.Config,
	row *ordereddict.Dict) error {

	mutation := &api_proto.HuntMutation{}
	mutation_cell, pres := row.Get("mutation")
	if !pres {
		return errors.New("No mutation")
	}

	err := utils.ParseIntoProtobuf(mutation_cell, mutation)
	if err != nil {
		return err
	}

	// Some mutations request the hunt be assigned an existing flow.
	if mutation.Assignment != nil {
		return self.directlyAssignFlow(ctx, config_obj, mutation)
	}

	return self.processMutation(ctx, config_obj, mutation)
}

// A Single function to update the hunt in the dispatcher. This only
// runs on the master node.
func (self *HuntManager) processMutation(
	ctx context.Context, config_obj *config_proto.Config,
	mutation *api_proto.HuntMutation) error {

	dispatcher, err := services.GetHuntDispatcher(config_obj)
	if err != nil {
		return err
	}

	modification := dispatcher.ModifyHuntObject(
		ctx, mutation.HuntId,
		func(hunt_obj *api_proto.Hunt) services.HuntModificationAction {
			modification := services.HuntUnmodified
			if hunt_obj == nil {
				return modification
			}

			if hunt_obj.Stats == nil {
				hunt_obj.Stats = &api_proto.HuntStats{}
			}

			if mutation.Stats == nil {
				mutation.Stats = &api_proto.HuntStats{}
			}

			// The following are very frequent modifications that
			// other frontends dont care about so we write them lazily
			// to the datastore.
			if mutation.Stats.TotalClientsScheduled > 0 {
				hunt_obj.Stats.TotalClientsScheduled +=
					mutation.Stats.TotalClientsScheduled

				modification = services.HuntFlushToDatastoreAsync
			}

			if mutation.Stats.TotalClientsWithResults > 0 {
				hunt_obj.Stats.TotalClientsWithResults +=
					mutation.Stats.TotalClientsWithResults

				modification = services.HuntFlushToDatastoreAsync
			}

			if mutation.Stats.TotalClientsWithErrors > 0 {
				hunt_obj.Stats.TotalClientsWithErrors +=
					mutation.Stats.TotalClientsWithErrors

				modification = services.HuntFlushToDatastoreAsync
			}

			// These modifications affect the state of the hunt and so
			// need to propagate to all minions
			// immediately. Eventually they will also hit the
			// filesystem too.
			if (mutation.State == api_proto.Hunt_STOPPED &&
				hunt_obj.State != api_proto.Hunt_STOPPED) ||
				(mutation.State == api_proto.Hunt_PAUSED &&
					hunt_obj.State != api_proto.Hunt_PAUSED) {

				hunt_obj.Stats.Stopped = true
				hunt_obj.State = api_proto.Hunt_STOPPED

				// Let all dispatchers know this hunt is stopped.
				modification = services.HuntPropagateChanges

			} else if mutation.State == api_proto.Hunt_RUNNING &&
				hunt_obj.State != api_proto.Hunt_RUNNING {
				hunt_obj.Stats.Stopped = false
				hunt_obj.State = api_proto.Hunt_RUNNING
				hunt_obj.StartTime = uint64(utils.GetTime().Now().UTC().UnixNano() / 1000)

				// This hunt is now started, let all dispatchers know
				// to participate connected clients.
				modification = services.HuntTriggerParticipation

			} else if mutation.State == api_proto.Hunt_ARCHIVED &&
				hunt_obj.State != api_proto.Hunt_ARCHIVED {

				hunt_obj.State = api_proto.Hunt_ARCHIVED

				// For archiving hunts we also send a notification to
				// this queue.
				row := ordereddict.NewDict().
					Set("Timestamp", utils.GetTime().Now().UTC().Unix()).
					Set("HuntId", mutation.HuntId).
					Set("User", mutation.User)

				// Alert listeners that the hunt is being archived.
				journal, err := services.GetJournal(config_obj)
				if err != nil {
					return services.HuntPropagateChanges
				}

				err = journal.PushRowsToArtifact(ctx, config_obj,
					[]*ordereddict.Dict{row}, "System.Hunt.Archive",
					"server", mutation.HuntId)
				if err != nil {
					return services.HuntPropagateChanges
				}

				modification = services.HuntPropagateChanges

				// Actually delete the hunt from disk - send all the
				// dispatchers the updated hunt object.
			} else if mutation.State == api_proto.Hunt_DELETED &&
				hunt_obj.State != api_proto.Hunt_DELETED {

				hunt_obj.State = api_proto.Hunt_DELETED

				modification = services.HuntPropagateChanges
			}

			if mutation.Description != "" {
				hunt_obj.HuntDescription = mutation.Description

				modification = services.HuntPropagateChanges
			}

			if len(mutation.Tags) > 0 &&
				!utils.StringSliceEq(mutation.Tags, hunt_obj.Tags) {

				// The "-" label is not valid and should never be
				// added. We use it to signify that labels should be
				// completely cleared.
				hunt_obj.Tags = utils.DeduplicateStringSlice(
					utils.FilterSlice(mutation.Tags, "-"))
				sort.Strings(hunt_obj.Tags)

				modification = services.HuntPropagateChanges
			}

			if mutation.Expires > 0 {
				hunt_obj.Expires = mutation.Expires

				modification = services.HuntPropagateChanges
			}

			// Hunt is restarted, notify all connected clients
			if mutation.StartTime > 0 {
				hunt_obj.StartTime = mutation.StartTime

				modification = services.HuntTriggerParticipation
			}

			return modification
		})

	// Force the dispatcher to write the index.
	if modification == services.HuntPropagateChanges {
		// return dispatcher.Refresh(ctx, config_obj)
	}

	return nil
}

// Check if the mutation requests a flow to be added to the hunt.
func (self *HuntManager) directlyAssignFlow(
	ctx context.Context,
	config_obj *config_proto.Config,
	mutation *api_proto.HuntMutation) error {
	assignment := mutation.Assignment
	if assignment == nil {
		return nil
	}

	// Verify the flow actually exists.
	launcher, err := services.GetLauncher(config_obj)
	if err != nil {
		return err
	}
	_, err = launcher.GetFlowDetails(
		ctx, config_obj, services.GetFlowOptions{},
		assignment.ClientId, assignment.FlowId)
	if err != nil {
		return err
	}

	// Append the flow to the client's table.
	journal, err := services.GetJournal(config_obj)
	if err != nil {
		return err
	}

	path_manager := paths.NewHuntPathManager(mutation.HuntId)
	err = journal.AppendToResultSet(config_obj,
		path_manager.Clients(), []*ordereddict.Dict{
			ordereddict.NewDict().
				Set("HuntId", mutation.HuntId).
				Set("ClientId", assignment.ClientId).
				Set("FlowId", assignment.FlowId).
				Set("Timestamp", utils.GetTime().Now().Unix()),
		}, services.JournalOptions{
			Sync: true,
		})
	if err != nil {
		return err
	}

	// Add this flow to the total.
	mutation.Stats = &api_proto.HuntStats{
		TotalClientsScheduled:   1,
		TotalClientsWithResults: 1,
	}

	return self.processMutation(ctx, config_obj, mutation)
}
