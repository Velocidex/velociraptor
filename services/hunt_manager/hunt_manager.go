/*
   Velociraptor - Hunting Evil
   Copyright (C) 2019 Velocidex Innovations.

   This program is free software: you can redistribute it and/or modify
   it under the terms of the GNU Affero General Public License as published
   by the Free Software Foundation, either version 3 of the License, or
   (at your option) any later version.

   This program is distributed in the hope that it will be useful,
   but WITHOUT ANY WARRANTY; without even the implied warranty of
   MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
   GNU Affero General Public License for more details.

   You should have received a copy of the GNU Affero General Public License
   along with this program.  If not, see <https://www.gnu.org/licenses/>.
*/
/*

  The hunt manager service only runs once on the master node and
  mediates updating the hunt objects in the data store. The hunt
  manager service watches for new clients added to a hunt and
  schedules new flows on them. It is written as a single thread so it
  is allowed to fall behind - it is not on the critical path and
  should be able to catch up with no problems.

  Hunt dispatching logic:

1) Client checks in with foreman. The foreman consults a read only
   copy of the hunts in the HuntDispatcher service to see if the
   client has run the hunt before.

2) If foreman decides client has not run this hunt, foreman pushes a
   message on the `System.Hunt.Participation` queue.

3) Hunt manager watches for new rows on System.Hunt.Participation and
   schedules collection on the client.

4) Hunt manager watches for flow completions and updates hunt stats re
   success or error of flow completion.

Note that steps 1 & 2 are on the critical path (and may be on a remote
frontend) and 3-4 are run on the master node.

*/

package hunt_manager

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/Velocidex/ordereddict"
	"github.com/golang/protobuf/proto"
	api_proto "www.velocidex.com/golang/velociraptor/api/proto"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	"www.velocidex.com/golang/velociraptor/constants"
	"www.velocidex.com/golang/velociraptor/datastore"
	"www.velocidex.com/golang/velociraptor/file_store"
	flows_proto "www.velocidex.com/golang/velociraptor/flows/proto"
	"www.velocidex.com/golang/velociraptor/logging"
	"www.velocidex.com/golang/velociraptor/paths"
	"www.velocidex.com/golang/velociraptor/services"
	"www.velocidex.com/golang/velociraptor/services/journal"
	"www.velocidex.com/golang/velociraptor/utils"
	vql_subsystem "www.velocidex.com/golang/velociraptor/vql"
	"www.velocidex.com/golang/vfilter"
)

// This is the record that will be sent by the foreman to the hunt
// manager.
type ParticipationRecord struct {
	HuntId      string `vfilter:"required,field=HuntId"`
	ClientId    string `vfilter:"required,field=ClientId"`
	Fqdn        string `vfilter:"optional,field=Fqdn"`
	FlowId      string `vfilter:"optional,field=FlowId"`
	Participate bool   `vfilter:"required,field=Participate"`
	Override    bool   `vfilter:"optional,field=Override"`
	Timestamp   uint64 `vfilter:"optional,field=Timestamp"`
	TS          uint64 `vfilter:"optional,field=_ts"`
}

type HuntManager struct {
	scope vfilter.Scope
}

func (self *HuntManager) Start(
	ctx context.Context,
	config_obj *config_proto.Config,
	wg *sync.WaitGroup) error {
	logger := logging.GetLogger(config_obj, &logging.FrontendComponent)
	logger.Info("<green>Starting</> the hunt manager service.")

	err := journal.WatchQueueWithCB(ctx, config_obj, wg,
		"Server.Internal.HuntModification", self.ProcessMutation)
	if err != nil {
		return err
	}

	err = journal.WatchQueueWithCB(ctx, config_obj, wg,
		"System.Hunt.Participation", self.ProcessParticipation)
	if err != nil {
		return err
	}

	err = journal.WatchQueueWithCB(ctx, config_obj, wg,
		"System.Flow.Completion", self.ProcessFlowCompletion)
	return err
}

// Modify a hunt object.
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

	dispatcher := services.GetHuntDispatcher()
	return dispatcher.ModifyHunt(mutation.HuntId,
		func(hunt_obj *api_proto.Hunt) error {
			if hunt_obj.Stats == nil {
				hunt_obj.Stats = &api_proto.HuntStats{}
			}

			if mutation.Stats == nil {
				mutation.Stats = &api_proto.HuntStats{}
			}

			hunt_obj.Stats.TotalClientsScheduled +=
				mutation.Stats.TotalClientsScheduled

			hunt_obj.Stats.TotalClientsWithResults +=
				mutation.Stats.TotalClientsWithResults

			// Have we stopped the hunt?
			if mutation.State == api_proto.Hunt_STOPPED ||
				mutation.State == api_proto.Hunt_PAUSED {
				hunt_obj.Stats.Stopped = true
				hunt_obj.State = api_proto.Hunt_STOPPED
			}

			if mutation.State == api_proto.Hunt_RUNNING {
				hunt_obj.Stats.Stopped = false
				hunt_obj.State = api_proto.Hunt_RUNNING
			}

			if mutation.State == api_proto.Hunt_ARCHIVED {
				hunt_obj.State = api_proto.Hunt_ARCHIVED
			}

			if mutation.Description != "" {
				hunt_obj.HuntDescription = mutation.Description
			}

			if mutation.StartTime > 0 {
				hunt_obj.StartTime = mutation.StartTime
			}

			return nil
		})
}

// Watch for all flows created by a hunt and maintain the list of hunt
// completions.
func (self *HuntManager) ProcessFlowCompletion(
	ctx context.Context,
	config_obj *config_proto.Config,
	row *ordereddict.Dict) error {

	flow := &flows_proto.ArtifactCollectorContext{}
	flow_any, _ := row.Get("Flow")
	err := utils.ParseIntoProtobuf(flow_any, flow)
	if err != nil {
		return err
	}

	if flow.Request == nil {
		return nil
	}

	hunt_id := flow.Request.Creator
	if !strings.HasPrefix(hunt_id, constants.HUNT_PREFIX) {
		return nil
	}

	// Flow is complete so add it to the hunt stats.
	dispatcher := services.GetHuntDispatcher()
	err = dispatcher.MutateHunt(config_obj,
		&api_proto.HuntMutation{
			HuntId: hunt_id,
			Stats: &api_proto.HuntStats{
				TotalClientsWithResults: 1,
			},
		})
	if err != nil {
		return err
	}

	path_manager := paths.NewHuntPathManager(hunt_id)
	return file_store.PushRows(config_obj, path_manager.ClientErrors(),
		[]*ordereddict.Dict{ordereddict.NewDict().
			Set("ClientId", flow.ClientId).
			Set("FlowId", flow.SessionId).
			Set("StartTime", time.Unix(0, int64(flow.CreateTime*1000))).
			Set("EndTime", time.Unix(0, int64(flow.ActiveTime*1000))).
			Set("Status", flow.State.String()).
			Set("Error", flow.Status)})
}

func (self *HuntManager) ProcessParticipation(
	ctx context.Context,
	config_obj *config_proto.Config,
	row *ordereddict.Dict) error {

	participation_row := &ParticipationRecord{}
	err := vfilter.ExtractArgs(self.scope, row, participation_row)
	if err != nil {
		return err
	}

	// The client will not participate in this hunt - nothing to do.
	if !participation_row.Participate {
		return nil
	}

	// Get some info about the client
	client_info_manager := services.GetClientInfoManager()
	if client_info_manager == nil {
		return nil
	}

	client_info, err := client_info_manager.Get(participation_row.ClientId)
	if err != nil {
		return fmt.Errorf("hunt_manager: failed to get client %v: %w",
			participation_row.ClientId, err)
	}

	err = checkHuntRanOnClient(config_obj, participation_row.ClientId,
		participation_row.HuntId)
	if err != nil {
		return err
	}

	// Get hunt information about this hunt.
	dispatcher := services.GetHuntDispatcher()
	if dispatcher == nil {
		return errors.New("hunt dispatcher invalid")
	}

	hunt_obj, pres := dispatcher.GetHunt(participation_row.HuntId)
	if !pres {
		return fmt.Errorf("Hunt %v not known", participation_row.HuntId)
	}

	// The event may override the regular hunt logic.
	if participation_row.Override {
		return scheduleHuntOnClient(ctx, config_obj,
			hunt_obj, participation_row.ClientId)
	}

	// Ignore stopped hunts.
	if hunt_obj.Stats.Stopped ||
		hunt_obj.State != api_proto.Hunt_RUNNING {
		return errors.New("hunt is stopped")

	} else if !huntMatchesOS(hunt_obj, client_info) {
		return errors.New("Hunt does not match OS condition")

		// Ignore hunts with label conditions which
		// exclude this client.

	} else if !huntHasLabel(config_obj, hunt_obj,
		participation_row.ClientId) {
		return errors.New("hunt label does not match")
	}

	// Hunt limit exceeded or it expired - we stop it.
	now := uint64(time.Now().UnixNano() / 1000)
	if (hunt_obj.ClientLimit > 0 &&
		hunt_obj.Stats.TotalClientsScheduled >= hunt_obj.ClientLimit) ||
		now > hunt_obj.Expires {

		// Hunt is expired, stop the hunt.
		return dispatcher.MutateHunt(config_obj,
			&api_proto.HuntMutation{
				HuntId: participation_row.HuntId,
				Stats:  &api_proto.HuntStats{Stopped: true}})
	}

	// Use hunt information to launch the flow against this
	// client.
	return scheduleHuntOnClient(ctx,
		config_obj, hunt_obj, participation_row.ClientId)
}

func StartHuntManager(
	ctx context.Context,
	wg *sync.WaitGroup,
	config_obj *config_proto.Config) error {

	manager, err := services.GetRepositoryManager()
	if err != nil {
		return err
	}

	result := &HuntManager{
		scope: manager.BuildScope(
			services.ScopeBuilder{
				Config: config_obj,
				Logger: logging.NewPlainLogger(config_obj, &logging.GenericComponent),
			}),
	}
	return result.Start(ctx, config_obj, wg)
}

// Check if the client should be scheduled based on required labels.
func huntHasLabel(
	config_obj *config_proto.Config,
	hunt_obj *api_proto.Hunt, client_id string) bool {
	labeler := services.GetLabeler()

	if hunt_obj.Condition == nil {
		return true
	}

	label_condition := hunt_obj.Condition.GetLabels()
	if label_condition == nil {
		return huntHasExcludeLabel(config_obj, hunt_obj, client_id)
	}

	for _, label := range label_condition.Label {
		if labeler.IsLabelSet(config_obj, client_id, label) {
			return huntHasExcludeLabel(config_obj, hunt_obj, client_id)
		}
	}

	return false
}

// Check if the client should be scheduled based on excluded labels.
func huntHasExcludeLabel(
	config_obj *config_proto.Config,
	hunt_obj *api_proto.Hunt, client_id string) bool {

	if hunt_obj.Condition == nil || hunt_obj.Condition.ExcludedLabels == nil {
		return true
	}

	labeler := services.GetLabeler()

	for _, label := range hunt_obj.Condition.ExcludedLabels.Label {
		if labeler.IsLabelSet(config_obj, client_id, label) {
			// Label is set on the client, it should be
			// excluded from the hunt.
			return false
		}
	}

	// Not excluded - schedule the client.
	return true
}

func huntMatchesOS(hunt_obj *api_proto.Hunt, client_info *services.ClientInfo) bool {
	if hunt_obj.Condition == nil {
		return true
	}
	os_condition := hunt_obj.Condition.GetOs()
	if os_condition == nil {
		return true
	}

	switch os_condition.Os {
	case api_proto.HuntOsCondition_WINDOWS:
		return client_info.OS == services.Windows
	case api_proto.HuntOsCondition_LINUX:
		return client_info.OS == services.Linux
	case api_proto.HuntOsCondition_OSX:
		return client_info.OS == services.MacOS
	}

	return true
}

// Check if we already launched it on this client. We maintain
// a data store index of all the clients and hunts to be able
// to quickly check if a certain hunt ran on a particular
// client. We dont care too much how fast this is because the
// hunt manager is running as an independent service and not
// in the critical path.
func checkHuntRanOnClient(
	config_obj *config_proto.Config,
	client_id, hunt_id string) error {
	db, err := datastore.GetDB(config_obj)
	if err != nil {
		return err
	}

	hunt_ids := []string{hunt_id}
	err = db.CheckIndex(
		config_obj, constants.HUNT_INDEX, client_id, hunt_ids)
	if err == nil {
		return errors.New("Client already ran this hunt")
	}

	err = db.SetIndex(
		config_obj, constants.HUNT_INDEX, client_id, hunt_ids)
	if err != nil {
		return fmt.Errorf("Setting hunt index: %w", err)
	}

	return nil
}

func scheduleHuntOnClient(
	ctx context.Context,
	config_obj *config_proto.Config,
	hunt_obj *api_proto.Hunt, client_id string) error {

	hunt_id := hunt_obj.HuntId

	manager, err := services.GetRepositoryManager()
	if err != nil {
		return err
	}

	repository, err := manager.GetGlobalRepository(config_obj)
	if err != nil {
		return err
	}

	launcher, err := services.GetLauncher()
	if err != nil {
		return err
	}

	// The request is pre-compiled into the hunt object.
	request := &flows_proto.ArtifactCollectorArgs{}
	proto.Merge(request, hunt_obj.StartRequest)

	// Direct the request against our client and schedule it.
	request.ClientId = client_id

	// Make sure the flow is created by the hunt - this is how we
	// track it.
	request.Creator = hunt_id

	flow_id, err := launcher.ScheduleArtifactCollection(
		ctx, config_obj, vql_subsystem.NullACLManager{}, repository, request)
	if err != nil {
		return err
	}

	// Append the row to the hunt so we can quickly query all
	// clients that belong on this hunt and their flow id.
	row := ordereddict.NewDict().
		Set("HuntId", hunt_id).
		Set("ClientId", client_id).
		Set("FlowId", flow_id).
		Set("Timestamp", time.Now().Unix())

	path_manager := paths.NewHuntPathManager(hunt_id)
	err = file_store.PushRows(config_obj,
		path_manager.Clients(), []*ordereddict.Dict{row})
	if err != nil {
		return err
	}

	// Modify the hunt stats.
	dispatcher := services.GetHuntDispatcher()
	if dispatcher == nil {
		return errors.New("hunt dispatcher invalid")
	}

	err = dispatcher.MutateHunt(config_obj,
		&api_proto.HuntMutation{
			HuntId: hunt_id,
			Stats: &api_proto.HuntStats{
				TotalClientsScheduled: 1}})
	if err != nil {
		return err
	}

	// Notify the client that the hunt applies to it.
	notifier := services.GetNotifier()
	if notifier != nil {
		_ = notifier.NotifyListener(config_obj, client_id)
	}

	return nil
}
