/*
   Velociraptor - Dig Deeper
   Copyright (C) 2019-2025 Rapid7 Inc.

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

Note that steps 1 & 2 are on the critical path (and may be on a minion
frontend) and 3-4 are run on the master node.

NOTE: The hunt manager does *not* interact with the datastore - all
datastore interactions are made through events sent to the hunt
dispatcher on the master node. On the minions the hunt dispatcher is
memory only.

*/

package hunt_manager

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/Velocidex/ordereddict"
	"golang.org/x/time/rate"
	"google.golang.org/protobuf/proto"
	api_proto "www.velocidex.com/golang/velociraptor/api/proto"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	flows_proto "www.velocidex.com/golang/velociraptor/flows/proto"
	"www.velocidex.com/golang/velociraptor/json"
	"www.velocidex.com/golang/velociraptor/logging"
	"www.velocidex.com/golang/velociraptor/paths"
	"www.velocidex.com/golang/velociraptor/services"
	"www.velocidex.com/golang/velociraptor/services/journal"
	"www.velocidex.com/golang/velociraptor/utils"
	"www.velocidex.com/golang/velociraptor/vql/acl_managers"
	"www.velocidex.com/golang/vfilter"
	"www.velocidex.com/golang/vfilter/arg_parser"
)

var (
	HuntManagerForTests *HuntManager
)

// This is the record that will be sent by the foreman to the hunt
// manager.
type ParticipationRecord struct {
	HuntId    string `vfilter:"required,field=HuntId"`
	ClientId  string `vfilter:"required,field=ClientId"`
	Fqdn      string `vfilter:"optional,field=Fqdn"`
	FlowId    string `vfilter:"optional,field=FlowId"`
	Override  bool   `vfilter:"optional,field=Override"`
	Timestamp uint64 `vfilter:"optional,field=Timestamp"`
	TS        uint64 `vfilter:"optional,field=_ts"`

	// Deprecated
	Participate bool `vfilter:"optional,field=Participate"`
}

type HuntManager struct {
	scope vfilter.Scope

	// Limits how quickly we schedule hunts. Should be fast enough
	// to be reasoable without overloading frontends
	limiter *rate.Limiter
}

func (self *HuntManager) Start(
	ctx context.Context,
	config_obj *config_proto.Config,
	wg *sync.WaitGroup) error {
	logger := logging.GetLogger(config_obj, &logging.FrontendComponent)
	logger.Info("<green>Starting</> hunt manager service for %v with rate limit %v/s.",
		services.GetOrgName(config_obj),
		config_obj.Frontend.Resources.NotificationsPerSecond)

	err := journal.WatchQueueWithCB(ctx, config_obj, wg,
		"Server.Internal.HuntModification",
		"HuntManager",
		self.ProcessMutation)
	if err != nil {
		return err
	}

	err = journal.WatchQueueWithCB(ctx, config_obj, wg,
		"System.Hunt.Participation",
		"HuntManager",
		self.ProcessParticipation)
	if err != nil {
		return err
	}

	err = journal.WatchQueueWithCB(ctx, config_obj, wg,
		"Server.Internal.Label", "HuntManager",
		self.ProcessLabelChange)
	if err != nil {
		return err
	}

	err = journal.WatchQueueWithCB(ctx, config_obj, wg,
		"Server.Internal.Interrogation", "HuntManager",
		self.ProcessInterrogation)
	if err != nil {
		return err
	}

	err = journal.WatchQueueWithCB(ctx, config_obj, wg,
		"System.Flow.Completion", "HuntManager",
		self.ProcessFlowCompletion)
	return err
}

// Watch for an interrogate completion and re-check all the hunts on
// this client in case the interrogate has more information (like an
// OS condition). This is important if a new client appears the
// foreman will attempt to participate it in the currrent hunt set but
// since we have not interrogated it yet we do not know information
// like OS, labels etc. Therefore we need to re-apply the hunts on the
// client again once we learn these.
func (self *HuntManager) ProcessInterrogation(
	ctx context.Context,
	config_obj *config_proto.Config,
	row *ordereddict.Dict) error {

	client_id, pres := row.GetString("ClientId")
	if !pres {
		return errors.New("ClientId not found")
	}

	return self.participateInRunningHunts(ctx, config_obj, client_id,
		// When a new client is interrogated, it can only really
		// affect hunts with OS conditions.
		func(hunt *api_proto.Hunt) bool {
			return hunt.Condition != nil &&
				hunt.Condition.GetOs() != nil
		})
}

// Watch for all flows created by a hunt and maintain the list of hunt
// completions.  TODO: This is inefficient because we are forced to
// open the flow object from disk to get at the request. We need to
// denote flows created by hunts by their own unique flow id.
func (self *HuntManager) ProcessFlowCompletion(
	ctx context.Context,
	config_obj *config_proto.Config,
	row *ordereddict.Dict) error {

	flow_any, pres := row.Get("Flow")
	if !pres {
		return nil
	}

	flow, ok := flow_any.(*flows_proto.ArtifactCollectorContext)
	if !ok || flow == nil {
		serialized, err := json.Marshal(flow_any)
		if err != nil {
			return err
		}
		flow = &flows_proto.ArtifactCollectorContext{}
		err = json.Unmarshal(serialized, flow)
		if err != nil {
			return err
		}
	}

	// Sessions IDs that come from a hunt have a special format with
	// the hunt id and flow id joined. This allows us to quickly
	// identify the flow that belongs to a hunt without needing to
	// read the original request from the datastore.
	flow_id, pres := row.GetString("FlowId")
	if !pres {
		return errors.New("FlowId not found")
	}

	hunt_id, ok := utils.ExtractHuntId(flow_id)
	if !ok {
		return nil
	}

	// Flow is complete so add it to the hunt stats. We send a
	// mutation to the hunt dispatcher to mediate internal hunt state
	// manipulation.
	mutation := &api_proto.HuntMutation{
		HuntId: hunt_id,
		Stats:  &api_proto.HuntStats{},
	}

	// All completions increment this counter.
	mutation.Stats.TotalClientsWithResults = 1

	// Only errored completions increment this one.
	if flow.State == flows_proto.ArtifactCollectorContext_ERROR {
		mutation.Stats.TotalClientsWithErrors = 1
	}

	// The minion hunt dispatcher does not actually care about flow
	// status, so we dont bother broadcasting a mutation for them. We
	// only need to update the local hunt dispatcher on the master
	// node which will flush to disk eventually.
	err := self.processMutation(ctx, config_obj, mutation)
	if err != nil {
		return err
	}

	journal, err := services.GetJournal(config_obj)
	if err != nil {
		return err
	}

	path_manager := paths.NewHuntPathManager(hunt_id)
	return journal.AppendToResultSet(config_obj, path_manager.ClientErrors(),
		[]*ordereddict.Dict{ordereddict.NewDict().
			Set("ClientId", flow.ClientId).
			Set("FlowId", flow.SessionId).
			Set("StartTime", time.Unix(0, int64(flow.StartTime*1000))).
			Set("EndTime", time.Unix(0, int64(flow.ActiveTime*1000))).
			Set("Status", flow.State.String()).
			Set("Error", flow.Status)}, services.JournalOptions{})
}

// When a label is changed we check all the active hunts to see if any
// of them are affected.
func (self *HuntManager) ProcessLabelChange(
	ctx context.Context,
	config_obj *config_proto.Config,
	row *ordereddict.Dict) error {

	client_id, pres := row.GetString("client_id")
	if !pres {
		return nil
	}

	// We only care when a label is added to a client.
	operation, pres := row.GetString("Operation")
	if !pres || operation != "Add" {
		return nil
	}

	return self.participateInRunningHunts(ctx, config_obj, client_id,
		// When a label changes it can only really affect hunts with
		// include label conditions.
		func(hunt *api_proto.Hunt) bool {
			return hunt.Condition != nil &&
				hunt.Condition.GetLabels() != nil
		})
}

func (self *HuntManager) participateInRunningHunts(ctx context.Context,
	config_obj *config_proto.Config, client_id string,
	should_participate_cb func(hunt *api_proto.Hunt) bool) error {

	journal, err := services.GetJournal(config_obj)
	if err != nil {
		return err
	}

	// Get hunt information about this hunt.
	dispatcher, err := services.GetHuntDispatcher(config_obj)
	if err != nil {
		return err
	}

	return dispatcher.ApplyFuncOnHunts(ctx, services.OnlyRunningHunts,
		func(hunt *api_proto.Hunt) error {
			if !should_participate_cb(hunt) {
				return nil
			}

			journal.PushRowsToArtifactAsync(ctx, config_obj,
				ordereddict.NewDict().
					Set("HuntId", hunt.HuntId).
					Set("ClientId", client_id), "System.Hunt.Participation")

			return nil
		})
}

// When a client is found to be missing a hunt, the foreman sends the
// participation message. We can examine this message and decide if
// the hunt really applies to this client.
func (self *HuntManager) ProcessParticipation(
	ctx context.Context,
	config_obj *config_proto.Config,
	row *ordereddict.Dict) error {

	// Ignore errors from the callback since they are not really
	// errors just reasons why the cliet should be ignored. There is
	// no need to log them.
	_ = self.ProcessParticipationWithError(ctx, config_obj, row)
	return nil
}

func (self *HuntManager) ProcessParticipationWithError(
	ctx context.Context,
	config_obj *config_proto.Config,
	row *ordereddict.Dict) error {

	participation_row := &ParticipationRecord{}
	err := arg_parser.ExtractArgsWithContext(
		ctx, self.scope, row, participation_row)
	if err != nil {
		logger := logging.GetLogger(config_obj, &logging.FrontendComponent)
		logger.Debug("ProcessParticipation: %v", err)
		return err
	}

	// Get some info about the client
	client_info_manager, err := services.GetClientInfoManager(config_obj)
	if err != nil {
		return err
	}

	client_info, err := client_info_manager.Get(ctx, participation_row.ClientId)
	if err != nil {
		return fmt.Errorf("hunt_manager: failed to get client %v: %w",
			participation_row.ClientId, err)
	}

	// If the hunt ran on the client already we just ignore
	// it. This is possible because the client may not have
	// updated its last hunt number in time to have a number of
	// hunt participation messages sent for it from different
	// frontends.
	err = checkHuntRanOnClient(config_obj, participation_row.ClientId,
		participation_row.HuntId)
	if err != nil {
		return fmt.Errorf("hunt_manager: %v already ran on client %v",
			participation_row.HuntId, participation_row.ClientId)
	}

	// Get hunt information about this hunt.
	dispatcher, err := services.GetHuntDispatcher(config_obj)
	if err != nil {
		return err
	}

	hunt_obj, pres := dispatcher.GetHunt(ctx, participation_row.HuntId)
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
		// Hunt is stopped.
		return fmt.Errorf("Hunt %v is stopped", participation_row.HuntId)

	} else if !huntMatchesOS(hunt_obj, client_info) {
		// Hunt does not match OS condition
		return fmt.Errorf("Hunt %v: %v does not match OS condition",
			participation_row.HuntId, participation_row.ClientId)

		// Ignore hunts with label conditions which
		// exclude this client.

	} else if !huntHasLabel(ctx, config_obj, hunt_obj,
		participation_row.ClientId) {
		return fmt.Errorf("Hunt %v: hunt label does not match with %v",
			participation_row.HuntId, participation_row.ClientId)
	}

	// Hunt limit exceeded or it expired - we stop it.
	now := uint64(utils.GetTime().Now().UnixNano() / 1000)
	if (hunt_obj.ClientLimit > 0 &&
		hunt_obj.Stats.TotalClientsScheduled >= hunt_obj.ClientLimit) ||
		now > hunt_obj.Expires {

		// Hunt is expired, stop the hunt.
		return dispatcher.MutateHunt(ctx, config_obj,
			&api_proto.HuntMutation{
				HuntId: participation_row.HuntId,
				Stats:  &api_proto.HuntStats{Stopped: true}})
	}

	// Control rate of hunt recruitment to balance server load.
	err = self.limiter.Wait(ctx)
	if err != nil {
		return err
	}

	// Use hunt information to launch the flow against this
	// client.
	return scheduleHuntOnClient(ctx,
		config_obj, hunt_obj, participation_row.ClientId)
}

func MakeHuntManager(config_obj *config_proto.Config) (*HuntManager, error) {
	manager, err := services.GetRepositoryManager(config_obj)
	if err != nil {
		return nil, err
	}

	return &HuntManager{
		limiter: rate.NewLimiter(rate.Limit(
			config_obj.Frontend.Resources.NotificationsPerSecond), 1),
		scope: manager.BuildScope(
			services.ScopeBuilder{
				Config: config_obj,
				Logger: logging.NewPlainLogger(config_obj, &logging.GenericComponent),
			}),
	}, nil
}

func NewHuntManager(
	ctx context.Context,
	wg *sync.WaitGroup,
	config_obj *config_proto.Config) error {

	result, err := MakeHuntManager(config_obj)
	if err != nil {
		return err
	}
	HuntManagerForTests = result

	return result.Start(ctx, config_obj, wg)
}

// Check if the client should be scheduled based on required labels.
func huntHasLabel(
	ctx context.Context,
	config_obj *config_proto.Config,
	hunt_obj *api_proto.Hunt, client_id string) bool {
	labeler := services.GetLabeler(config_obj)

	if hunt_obj.Condition == nil {
		return true
	}

	label_condition := hunt_obj.Condition.GetLabels()
	if label_condition == nil {
		return huntHasExcludeLabel(ctx, config_obj, hunt_obj, client_id)
	}

	for _, label := range label_condition.Label {
		if labeler.IsLabelSet(ctx, config_obj, client_id, label) {
			return huntHasExcludeLabel(ctx, config_obj, hunt_obj, client_id)
		}
	}

	return false
}

// Check if the client should be scheduled based on excluded labels.
func huntHasExcludeLabel(
	ctx context.Context,
	config_obj *config_proto.Config,
	hunt_obj *api_proto.Hunt, client_id string) bool {

	if hunt_obj.Condition == nil || hunt_obj.Condition.ExcludedLabels == nil {
		return true
	}

	labeler := services.GetLabeler(config_obj)

	for _, label := range hunt_obj.Condition.ExcludedLabels.Label {
		if labeler.IsLabelSet(ctx, config_obj, client_id, label) {
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

	os := client_info.OS()
	switch os_condition.Os {
	case api_proto.HuntOsCondition_WINDOWS:
		return os == services.Windows
	case api_proto.HuntOsCondition_LINUX:
		return os == services.Linux
	case api_proto.HuntOsCondition_OSX:
		return os == services.MacOS
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

	indexer, err := services.GetIndexer(config_obj)
	if err != nil {
		return err
	}

	hunt_ids := []string{hunt_id}
	err = indexer.CheckSimpleIndex(
		config_obj, paths.HUNT_INDEX, client_id, hunt_ids)
	if err == nil {
		return errors.New("Client already ran this hunt")
	}

	return nil
}

func setHuntRanOnClient(config_obj *config_proto.Config,
	client_id, hunt_id string) error {

	indexer, err := services.GetIndexer(config_obj)
	if err != nil {
		return err
	}

	hunt_ids := []string{hunt_id}
	err = indexer.SetSimpleIndex(
		config_obj, paths.HUNT_INDEX, client_id, hunt_ids)
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

	manager, err := services.GetRepositoryManager(config_obj)
	if err != nil {
		return err
	}

	repository, err := manager.GetGlobalRepository(config_obj)
	if err != nil {
		return err
	}

	launcher, err := services.GetLauncher(config_obj)
	if err != nil {
		return err
	}

	// The request is pre-compiled into the hunt object.
	request := proto.Clone(hunt_obj.StartRequest).(*flows_proto.ArtifactCollectorArgs)

	// Direct the request against our client and schedule it.
	request.ClientId = client_id

	flow_id, err := launcher.ScheduleArtifactCollection(
		ctx, config_obj, acl_managers.NullACLManager{},
		repository, request, nil)
	if err != nil {
		return err
	}

	journal, err := services.GetJournal(config_obj)
	if err != nil {
		return err
	}

	// Append the row to the hunt so we can quickly query all
	// clients that belong on this hunt and their flow id.
	row := ordereddict.NewDict().
		Set("HuntId", hunt_id).
		Set("ClientId", client_id).
		Set("FlowId", flow_id).
		Set("Timestamp", utils.GetTime().Now().Unix())

	path_manager := paths.NewHuntPathManager(hunt_id)
	err = journal.AppendToResultSet(config_obj,
		path_manager.Clients(), []*ordereddict.Dict{row},
		services.JournalOptions{})
	if err != nil {
		return err
	}

	// Modify the hunt stats.
	dispatcher, err := services.GetHuntDispatcher(config_obj)
	if err != nil {
		return err
	}

	err = dispatcher.MutateHunt(ctx, config_obj,
		&api_proto.HuntMutation{
			HuntId: hunt_id,
			Stats: &api_proto.HuntStats{
				TotalClientsScheduled: 1}})
	if err != nil {
		return err
	}

	err = setHuntRanOnClient(config_obj, client_id, hunt_id)
	if err != nil {
		return err
	}

	return nil
}
