// +build deprecated

// This flow is deprecated in favor of artifacts.

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
package flows

import (
	"errors"
	"path"

	"github.com/golang/protobuf/proto"
	"github.com/golang/protobuf/ptypes"
	actions_proto "www.velocidex.com/golang/velociraptor/actions/proto"
	api_proto "www.velocidex.com/golang/velociraptor/api/proto"
	artifacts "www.velocidex.com/golang/velociraptor/artifacts"
	constants "www.velocidex.com/golang/velociraptor/constants"
	crypto_proto "www.velocidex.com/golang/velociraptor/crypto/proto"
	flows_proto "www.velocidex.com/golang/velociraptor/flows/proto"
)

const (
	_                          = iota
	processVQLResponses uint64 = iota
)

type VQLCollector struct {
	*BaseFlow
}

func (self *VQLCollector) New() Flow {
	return &VQLCollector{&BaseFlow{}}
}

func (self *VQLCollector) Start(
	config_obj *api_proto.Config,
	flow_obj *AFF4FlowObject,
	args proto.Message) error {
	vql_collector_args, ok := args.(*actions_proto.VQLCollectorArgs)
	if !ok {
		return errors.New("expected args of type VQLCollectorArgs")
	}

	// Add any required artifacts to the request.
	repository, err := artifacts.GetGlobalRepository(config_obj)
	if err != nil {
		return err
	}
	err = repository.PopulateArtifactsVQLCollectorArgs(vql_collector_args)
	if err != nil {
		return err
	}

	return QueueMessageForClient(
		config_obj, flow_obj,
		"VQLClientAction",
		vql_collector_args, processVQLResponses)
}

func (self *VQLCollector) ProcessMessage(
	config_obj *api_proto.Config,
	flow_obj *AFF4FlowObject,
	message *crypto_proto.GrrMessage) error {
	err := flow_obj.FailIfError(config_obj, message)
	if err != nil {
		return err
	}

	switch message.RequestId {
	case processVQLResponses:
		if flow_obj.IsRequestComplete(message) {
			return flow_obj.Complete(config_obj)
		}

		err = StoreResultInFlow(config_obj, flow_obj, message)
		if err != nil {
			return err
		}

		// Receive any file upload the client sent.
	case constants.TransferWellKnownFlowId:
		return appendDataToFile(
			config_obj, flow_obj,
			path.Join("clients",
				flow_obj.RunnerArgs.ClientId,
				"uploads",
				path.Base(message.SessionId)),
			message)
	}
	return nil
}

func init() {
	impl := VQLCollector{}
	default_args, _ := ptypes.MarshalAny(&actions_proto.VQLCollectorArgs{})
	desc := &flows_proto.FlowDescriptor{
		Name:         "VQLCollector",
		FriendlyName: "VQL Collector",
		Category:     "Collectors",
		Doc:          "Issues VQL queries to the Velociraptor client and collects the results.",
		ArgsType:     "VQLCollectorArgs",
		DefaultArgs:  default_args,
	}

	RegisterImplementation(desc, &impl)
}
