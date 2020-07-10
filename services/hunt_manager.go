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
   Hunt dispatching logic:

1) Client checks in with foreman.

2) If foreman decides client has not run this hunt, foreman spaces a
   message on `System.Hunt.Participation`.

3) Hunt manager watches for new rows on System.Hunt.Participation and
   schedules collection.

4) Hunt manager watches for flow completions and updates hunt stats re
   success or error of flow completion.
*/

package services

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
	"www.velocidex.com/golang/velociraptor/artifacts"
	"www.velocidex.com/golang/velociraptor/clients"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	"www.velocidex.com/golang/velociraptor/constants"
	"www.velocidex.com/golang/velociraptor/datastore"
	flows_proto "www.velocidex.com/golang/velociraptor/flows/proto"
	"www.velocidex.com/golang/velociraptor/logging"
	"www.velocidex.com/golang/velociraptor/paths"
	"www.velocidex.com/golang/velociraptor/utils"
	"www.velocidex.com/golang/vfilter"
)

type ParticipationRecord struct {
	HuntId      string `vfilter:"required,field=HuntId"`
	ClientId    string `vfilter:"required,field=ClientId"`
	Fqdn        string `vfilter:"optional,field=Fqdn"`
	FlowId      string `vfilter:"optional,field=FlowId"`
	Participate bool   `vfilter:"required,field=Participate"`
	Timestamp   uint64 `vfilter:"optional,field=Timestamp"`
	TS          uint64 `vfilter:"optional,field=_ts"`
}

type HuntManager struct {
	mu sync.Mutex

	config_obj *config_proto.Config
}

func (self *HuntManager) Start(
	ctx context.Context,
	wg *sync.WaitGroup) error {
	logger := logging.GetLogger(self.config_obj, &logging.FrontendComponent)
	logger.Info("Starting the hunt manager service.")

	err := self.StartParticipation(ctx, wg)
	if err != nil {
		return err
	}

	return self.StartFlowCompletion(ctx, wg)
}

// Watch the Participation queue and schedule new collections.
func (self *HuntManager) StartParticipation(
	ctx context.Context,
	wg *sync.WaitGroup) error {

	scope := vfilter.NewScope()
	qm_chan, cancel := GetJournal().Watch("System.Hunt.Participation")

	wg.Add(1)
	go func() {
		defer cancel()
		defer wg.Done()
		defer self.Close()

		for {
			select {
			case row, ok := <-qm_chan:
				if !ok {
					return
				}
				self.ProcessRow(ctx, scope, row)

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
	wg *sync.WaitGroup) error {

	scope := vfilter.NewScope()
	qm_chan, cancel := GetJournal().Watch("System.Flow.Completion")

	wg.Add(1)
	go func() {
		defer cancel()
		defer wg.Done()
		defer self.Close()

		for {
			select {
			case row, ok := <-qm_chan:
				if !ok {
					return
				}
				self.ProcessFlowCompletion(ctx, scope, row)

			case <-ctx.Done():
				return
			}
		}
	}()

	return nil
}

// Close will block until all our cleanup is done.
func (self *HuntManager) Close() {
	self.mu.Lock()
	defer self.mu.Unlock()
	logger := logging.GetLogger(self.config_obj, &logging.FrontendComponent)
	logger.Info("Shutting down hunt manager service.")
}

func (self *HuntManager) ProcessFlowCompletion(
	ctx context.Context,
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
	err = GetJournal().PushRows(path_manager.ClientErrors(),
		[]*ordereddict.Dict{ordereddict.NewDict().
			Set("ClientId", flow.ClientId).
			Set("FlowId", flow.SessionId).
			Set("StartTime", time.Unix(0, int64(flow.CreateTime*1000))).
			Set("EndTime", time.Unix(0, int64(flow.KillTimestamp*1000))).
			Set("Status", flow.State.String()).
			Set("Error", flow.Status)})
	if err != nil {
		logger := logging.GetLogger(self.config_obj, &logging.FrontendComponent)
		logger.Error(fmt.Sprintf("ProcessFlowCompletion: %v", err))
	}
}

func (self *HuntManager) ProcessRow(
	ctx context.Context,
	scope *vfilter.Scope,
	row *ordereddict.Dict) {
	self.mu.Lock()
	defer self.mu.Unlock()

	participation_row := &ParticipationRecord{}
	err := vfilter.ExtractArgs(scope, row, participation_row)
	if err != nil {
		scope.Log("ExtractArgs %v", err)
		return
	}

	// The client will not participate in this hunt - nothing to do.
	if !participation_row.Participate {
		return
	}

	// Check if we already launched it on this client. We maintain
	// a data store index of all the clients and hunts to be able
	// to quickly check if a certain hunt ran on a particular
	// client. We dont care too much how fast this is because the
	// hunt manager is running as an independent service and not
	// in the critical path.
	db, err := datastore.GetDB(self.config_obj)
	if err != nil {
		return
	}

	hunt_ids := []string{participation_row.HuntId}
	err = db.CheckIndex(self.config_obj, constants.HUNT_INDEX,
		participation_row.ClientId, hunt_ids)
	if err == nil {
		return
	}

	err = db.SetIndex(self.config_obj, constants.HUNT_INDEX,
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
	err = GetHuntDispatcher().ModifyHunt(
		participation_row.HuntId,
		func(hunt_obj *api_proto.Hunt) error {
			// Ignore stopped hunts.
			if hunt_obj.Stats.Stopped ||
				hunt_obj.State != api_proto.Hunt_RUNNING {
				return errors.New("hunt is stopped")
			}

			// Ignore hunts with label conditions which
			// exclude this client.
			has_label, err := huntHasLabel(
				self.config_obj,
				hunt_obj,
				participation_row.ClientId)
			if err != nil {
				return err
			}

			if !has_label {
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
		scope.Log("hunt manager: launching %v:  %v", participation_row, err)
		return
	}

	repository, err := artifacts.GetGlobalRepository(self.config_obj)
	if err != nil {
		scope.Log("hunt manager: launching %v:  %v", participation_row, err)
		return
	}

	flow_id, err := ScheduleArtifactCollection(ctx, self.config_obj,
		"Server", repository, request)
	if err != nil {
		scope.Log("hunt manager: %s", err.Error())
		return
	}

	row.Set("FlowId", flow_id)
	row.Set("Timestamp", time.Now().Unix())

	path_manager := paths.NewHuntPathManager(participation_row.HuntId)
	GetJournal().PushRows(path_manager.Clients(), []*ordereddict.Dict{row})

	// Notify the client
	err = NotifyListener(self.config_obj, participation_row.ClientId)
	if err != nil {
		scope.Log("hunt manager: %v", err)
	}
}

func startHuntManager(
	ctx context.Context,
	wg *sync.WaitGroup,
	config_obj *config_proto.Config) (*HuntManager, error) {
	result := &HuntManager{
		config_obj: config_obj,
	}
	return result, result.Start(ctx, wg)
}

func huntHasLabel(config_obj *config_proto.Config,
	hunt_obj *api_proto.Hunt,
	client_id string) (bool, error) {

	label_condition := hunt_obj.Condition.GetLabels()
	if label_condition != nil && len(label_condition.Label) > 0 {
		request := &api_proto.LabelClientsRequest{
			ClientIds: []string{client_id},
			Labels:    label_condition.Label,
			Operation: "check",
		}

		err := clients.LabelClients(config_obj, request)
		if err != nil {
			return false, nil
		}
	}

	return true, nil
}
