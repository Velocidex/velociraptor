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

  The hunt manager service should only be run once across the entire
  deployment.  The hunt manager service watches for new clients added
  to a hunt and schedules new flows on them. It is written as a single
  thread so it is allowed to fall behind - it is not on the critical
  path and should be able to catch up with no problems.

  Hunt dispatching logic:

1) Client checks in with foreman.

2) If foreman decides client has not run this hunt, foreman pushes a
   message on the `System.Hunt.Participation` queue.

3) Hunt manager watches for new rows on System.Hunt.Participation and
   schedules collection.

4) Hunt manager watches for flow completions and updates hunt stats re
   success or error of flow completion.

Note that steps 1 & 2 are on the critical path and 3-4 are not.

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
	flows_proto "www.velocidex.com/golang/velociraptor/flows/proto"
	"www.velocidex.com/golang/velociraptor/logging"
	"www.velocidex.com/golang/velociraptor/paths"
	"www.velocidex.com/golang/velociraptor/services"
	"www.velocidex.com/golang/velociraptor/utils"
	vql_subsystem "www.velocidex.com/golang/velociraptor/vql"
	"www.velocidex.com/golang/vfilter"
)

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

type HuntManager struct{}

func (self *HuntManager) Start(
	ctx context.Context,
	config_obj *config_proto.Config,
	wg *sync.WaitGroup) error {
	logger := logging.GetLogger(config_obj, &logging.FrontendComponent)
	logger.Info("<green>Starting</> the hunt manager service.")

	err := self.StartParticipation(ctx, config_obj, wg)
	if err != nil {
		return err
	}

	return self.StartFlowCompletion(ctx, config_obj, wg)
}

// Watch the Participation queue and schedule new collections.
func (self *HuntManager) StartParticipation(
	ctx context.Context,
	config_obj *config_proto.Config,
	wg *sync.WaitGroup) error {

	manager, err := services.GetRepositoryManager()
	if err != nil {
		return err
	}

	scope := manager.BuildScope(
		services.ScopeBuilder{
			Config: config_obj,
			Logger: logging.NewPlainLogger(config_obj, &logging.GenericComponent),
		})
	journal, err := services.GetJournal()
	if err != nil {
		return err
	}
	qm_chan, cancel := journal.Watch("System.Hunt.Participation")

	wg.Add(1)
	go func() {
		defer cancel()
		defer wg.Done()

		for {
			select {
			case row, ok := <-qm_chan:
				if !ok {
					return
				}
				self.ProcessRow(ctx, config_obj, scope, row)

			case <-ctx.Done():
				return
			}
		}
	}()

	return nil
}

// Watch the Flow.Completion queue and report status.
func (self *HuntManager) StartFlowCompletion(
	ctx context.Context,
	config_obj *config_proto.Config,
	wg *sync.WaitGroup) error {

	scope := vfilter.NewScope()
	journal, err := services.GetJournal()
	if err != nil {
		return err
	}

	qm_chan, cancel := journal.Watch("System.Flow.Completion")

	wg.Add(1)
	go func() {
		defer cancel()
		defer wg.Done()

		for {
			select {
			case row, ok := <-qm_chan:
				if !ok {
					return
				}
				self.ProcessFlowCompletion(ctx, config_obj, scope, row)

			case <-ctx.Done():
				return
			}
		}
	}()

	return nil
}

func (self *HuntManager) ProcessFlowCompletion(
	ctx context.Context,
	config_obj *config_proto.Config,
	scope *vfilter.Scope,
	row *ordereddict.Dict) {

	flow := &flows_proto.ArtifactCollectorContext{}
	flow_any, _ := row.Get("Flow")
	err := utils.ParseIntoProtobuf(flow_any, flow)
	if err != nil {
		return
	}

	if flow.Request == nil {
		return
	}

	hunt_id := flow.Request.Creator
	if !strings.HasPrefix(hunt_id, constants.HUNT_PREFIX) {
		return
	}

	path_manager := paths.NewHuntPathManager(hunt_id)
	journal, err := services.GetJournal()
	if err != nil {
		return
	}

	err = journal.PushRows(config_obj, path_manager.ClientErrors(),
		[]*ordereddict.Dict{ordereddict.NewDict().
			Set("ClientId", flow.ClientId).
			Set("FlowId", flow.SessionId).
			Set("StartTime", time.Unix(0, int64(flow.CreateTime*1000))).
			Set("EndTime", time.Unix(0, int64(flow.ActiveTime*1000))).
			Set("Status", flow.State.String()).
			Set("Error", flow.Status)})
	if err != nil {
		logger := logging.GetLogger(config_obj, &logging.FrontendComponent)
		logger.Error(fmt.Sprintf("ProcessFlowCompletion: %v", err))
	}
}

func (self *HuntManager) ProcessRow(
	ctx context.Context,
	config_obj *config_proto.Config,
	scope *vfilter.Scope,
	row *ordereddict.Dict) {

	participation_row := &ParticipationRecord{}
	err := vfilter.ExtractArgs(scope, row, participation_row)
	if err != nil {
		scope.Log("hunt_manager: %v", err)
		return
	}

	// The client will not participate in this hunt - nothing to do.
	if !participation_row.Participate {
		return
	}

	// Get some info about the client
	client_info_manager := services.GetClientInfoManager()
	if client_info_manager == nil {
		return
	}

	client_info, err := client_info_manager.Get(participation_row.ClientId)
	if err != nil {
		scope.Log("hunt_manager: failed to get client %v: %v",
			participation_row.ClientId, err)
		return
	}

	// Check if we already launched it on this client. We maintain
	// a data store index of all the clients and hunts to be able
	// to quickly check if a certain hunt ran on a particular
	// client. We dont care too much how fast this is because the
	// hunt manager is running as an independent service and not
	// in the critical path.
	db, err := datastore.GetDB(config_obj)
	if err != nil {
		return
	}

	hunt_ids := []string{participation_row.HuntId}
	err = db.CheckIndex(config_obj, constants.HUNT_INDEX,
		participation_row.ClientId, hunt_ids)
	if err == nil {
		return
	}

	err = db.SetIndex(config_obj, constants.HUNT_INDEX,
		participation_row.ClientId, hunt_ids)
	if err != nil {
		scope.Log("Setting hunt index: %v", err)
		return
	}

	request := &flows_proto.ArtifactCollectorArgs{
		ClientId: participation_row.ClientId,
		Creator:  participation_row.HuntId,
	}

	// Get hunt information about this hunt.
	now := uint64(time.Now().UnixNano() / 1000)
	err = services.GetHuntDispatcher().ModifyHunt(
		participation_row.HuntId,
		func(hunt_obj *api_proto.Hunt) error {
			if hunt_obj.Stats == nil {
				hunt_obj.Stats = &api_proto.HuntStats{}
			}

			// The event may override the regular hunt
			// logic.
			if participation_row.Override {
				proto.Merge(request, hunt_obj.StartRequest)
				hunt_obj.Stats.TotalClientsScheduled += 1

				return nil
			}

			// Ignore stopped hunts.
			if hunt_obj.Stats.Stopped ||
				hunt_obj.State != api_proto.Hunt_RUNNING {
				return errors.New("hunt is stopped")
			}

			if !huntMatchesOS(hunt_obj, client_info) {
				return errors.New("Hunt does not match OS condition")
			}

			// Ignore hunts with label conditions which
			// exclude this client.
			if !huntHasLabel(config_obj, hunt_obj,
				participation_row.ClientId) {
				return errors.New("hunt label does not match")
			}

			// Hunt limit exceeded or it expired - we stop it.
			if (hunt_obj.ClientLimit > 0 &&
				hunt_obj.Stats.TotalClientsScheduled >= hunt_obj.ClientLimit) ||
				now > hunt_obj.Expires {

				// Stop the hunt.
				hunt_obj.Stats.Stopped = true
				return errors.New("hunt is expired")
			}

			// Use hunt information to launch the flow
			// against this client.
			proto.Merge(request, hunt_obj.StartRequest)
			hunt_obj.Stats.TotalClientsScheduled += 1

			return nil
		})

	if err != nil {
		return
	}

	manager, err := services.GetRepositoryManager()
	if err != nil {
		scope.Log("hunt_manager: %v", err)
		return
	}

	repository, err := manager.GetGlobalRepository(config_obj)
	if err != nil {
		scope.Log("hunt manager: GetGlobalRepository: %v", err)
		return
	}

	launcher, err := services.GetLauncher()
	if err != nil {
		return
	}

	// Direct the request against our client and schedule it.
	request.ClientId = participation_row.ClientId
	flow_id, err := launcher.ScheduleArtifactCollection(
		ctx, config_obj, vql_subsystem.NullACLManager{}, repository, request)
	if err != nil {
		scope.Log("hunt manager: %v", err)
		return
	}

	row.Set("FlowId", flow_id)
	row.Set("Timestamp", time.Now().Unix())
	journal, err := services.GetJournal()
	if err != nil {
		scope.Log("hunt manager: %v", err)
		return
	}

	path_manager := paths.NewHuntPathManager(participation_row.HuntId)
	err = journal.PushRows(config_obj,
		path_manager.Clients(), []*ordereddict.Dict{row})
	if err != nil {
		scope.Log("hunt manager: %v", err)
		return
	}

	// Notify the client
	notifier := services.GetNotifier()
	if notifier != nil {
		err = services.GetNotifier().NotifyListener(
			config_obj, participation_row.ClientId)
		if err != nil {
			scope.Log("hunt manager: %v", err)
		}
	}
}

func StartHuntManager(
	ctx context.Context,
	wg *sync.WaitGroup,
	config_obj *config_proto.Config) error {

	result := &HuntManager{}
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
