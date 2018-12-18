// The foreman is a Well Known Flow for clients to check for hunt
// memberships. Each client periodically informs the foreman on the
// most recent hunt it executed, and the foreman launches the relevant
// flow on the client.

// The process goes like this:

// 1. The client sends a message to the foreman periodically with the
//    timestamp of the most recent hunt it ran.
// 2. The foreman then starts a conditional flow for each client. The
//    conditional flow runs a VQL query on the client and determines
//    if another flow should be run based on a codition posed on the
//    VQL response.
// 3. If the condition is satisfied, the flow proceeds to run

package flows

import (
	"context"

	"github.com/golang/protobuf/ptypes"
	errors "github.com/pkg/errors"
	actions_proto "www.velocidex.com/golang/velociraptor/actions/proto"
	api_proto "www.velocidex.com/golang/velociraptor/api/proto"
	artifacts "www.velocidex.com/golang/velociraptor/artifacts"
	constants "www.velocidex.com/golang/velociraptor/constants"
	crypto_proto "www.velocidex.com/golang/velociraptor/crypto/proto"
	flows_proto "www.velocidex.com/golang/velociraptor/flows/proto"
	"www.velocidex.com/golang/velociraptor/grpc_client"
	"www.velocidex.com/golang/velociraptor/responder"
	urns "www.velocidex.com/golang/velociraptor/urns"
)

type Foreman struct {
	BaseFlow
}

func (self *Foreman) New() Flow {
	return &Foreman{BaseFlow{}}
}

func (self *Foreman) ProcessEventTables(
	config_obj *api_proto.Config,
	flow_obj *AFF4FlowObject,
	source string,
	arg *actions_proto.ForemanCheckin) error {

	// Need to update client's event table.
	if arg.LastEventTableVersion < config_obj.Events.Version {
		repository, err := artifacts.GetGlobalRepository(config_obj)
		if err != nil {
			return err
		}

		event_table := &actions_proto.VQLEventTable{
			Version: config_obj.Events.Version,
		}
		for _, name := range config_obj.Events.Artifacts {
			vql_collector_args := &actions_proto.VQLCollectorArgs{}
			artifact, pres := repository.Get(name)
			if !pres {
				return errors.New("Unknown artifact " + name)
			}

			err := artifacts.Compile(artifact, vql_collector_args)
			if err != nil {
				return err
			}
			// Add any artifact dependencies.
			repository.PopulateArtifactsVQLCollectorArgs(vql_collector_args)
			event_table.Event = append(event_table.Event, vql_collector_args)
		}

		channel := grpc_client.GetChannel(config_obj)
		defer channel.Close()

		flow_runner_args := &flows_proto.FlowRunnerArgs{
			ClientId: source,
			FlowName: "MonitoringFlow",
		}
		flow_args, err := ptypes.MarshalAny(event_table)
		if err != nil {
			return errors.WithStack(err)
		}
		flow_runner_args.Args = flow_args
		client := api_proto.NewAPIClient(channel)
		_, err = client.LaunchFlow(context.Background(), flow_runner_args)
		if err != nil {
			return err
		}
	}

	return nil
}

func (self *Foreman) ProcessMessage(
	config_obj *api_proto.Config,
	flow_obj *AFF4FlowObject,
	message *crypto_proto.GrrMessage) error {
	foreman_checkin, ok := responder.ExtractGrrMessagePayload(
		message).(*actions_proto.ForemanCheckin)
	if !ok {
		return errors.New("Expected args of type ForemanCheckin")
	}

	err := self.ProcessEventTables(config_obj, flow_obj, message.Source,
		foreman_checkin)
	if err != nil {
		return err
	}

	hunts_dispatcher, err := GetHuntDispatcher(config_obj)
	if err != nil {
		return err
	}

	for _, hunt := range hunts_dispatcher.GetApplicableHunts(
		foreman_checkin.LastHuntTimestamp) {

		flow_condition_query, err := calculateFlowConditionQuery(hunt)
		if err != nil {
			return err
		}

		urn := urns.BuildURN(
			"clients", message.Source,
			"flows", constants.MONITORING_WELL_KNOWN_FLOW)

		err = QueueAndNotifyClient(
			config_obj, message.Source, urn,
			"VQLClientAction",
			flow_condition_query,
			processVQLResponses)
		if err != nil {
			return err
		}

		err = QueueAndNotifyClient(
			config_obj, message.Source, urn,
			"UpdateForeman",
			&actions_proto.ForemanCheckin{
				LastHuntTimestamp: hunt.CreateTime,
			}, processUpgradeForeman)
		if err != nil {
			return err
		}
	}
	return nil
}

func calculateFlowConditionQuery(hunt *api_proto.Hunt) (
	*actions_proto.VQLCollectorArgs, error) {

	return getDefaultCollectorArgs(hunt.HuntId), nil
}

func getDefaultCollectorArgs(hunt_id string) *actions_proto.VQLCollectorArgs {
	return &actions_proto.VQLCollectorArgs{
		Env: []*actions_proto.VQLEnv{
			&actions_proto.VQLEnv{
				Key:   "HuntId",
				Value: hunt_id,
			},
		},
		Query: []*actions_proto.VQLRequest{
			&actions_proto.VQLRequest{
				Name: "Artifact System.Hunt.Participation",
				VQL: "SELECT now() as Timestamp, HuntId, " +
					"true as Participate from info()",
			},
		},
	}
}
