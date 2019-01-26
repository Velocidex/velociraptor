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
	"encoding/json"
	"fmt"
	"io/ioutil"
	"path"

	"github.com/golang/protobuf/proto"
	"github.com/golang/protobuf/ptypes"
	errors "github.com/pkg/errors"
	"github.com/sirupsen/logrus"
	actions_proto "www.velocidex.com/golang/velociraptor/actions/proto"
	api_proto "www.velocidex.com/golang/velociraptor/api/proto"
	artifacts "www.velocidex.com/golang/velociraptor/artifacts"
	constants "www.velocidex.com/golang/velociraptor/constants"
	crypto_proto "www.velocidex.com/golang/velociraptor/crypto/proto"
	"www.velocidex.com/golang/velociraptor/file_store"
	"www.velocidex.com/golang/velociraptor/file_store/csv"
	flows_proto "www.velocidex.com/golang/velociraptor/flows/proto"
	"www.velocidex.com/golang/velociraptor/logging"
	"www.velocidex.com/golang/velociraptor/responder"
	vql_subsystem "www.velocidex.com/golang/velociraptor/vql"
	"www.velocidex.com/golang/vfilter"
)

type ArtifactCollector struct {
	*VQLCollector
}

func (self *ArtifactCollector) New() Flow {
	return &ArtifactCollector{&VQLCollector{}}
}

func (self *ArtifactCollector) Start(
	config_obj *api_proto.Config,
	flow_obj *AFF4FlowObject,
	args proto.Message) error {
	collector_args, ok := args.(*flows_proto.ArtifactCollectorArgs)
	if !ok {
		return errors.New("Expected args of type ArtifactCollectorArgs")
	}

	if collector_args.Artifacts == nil || len(collector_args.Artifacts.Names) == 0 {
		return errors.New("Some artifacts to run are required.")
	}

	repository, err := artifacts.GetGlobalRepository(config_obj)
	if err != nil {
		return err
	}

	// Update the flow's artifacts list.
	flow_obj.FlowContext.Artifacts = collector_args.Artifacts.Names
	flow_obj.SetContext(flow_obj.FlowContext)

	vql_collector_args := &actions_proto.VQLCollectorArgs{
		OpsPerSecond: collector_args.OpsPerSecond,
	}
	for _, name := range collector_args.Artifacts.Names {
		artifact, pres := repository.Get(name)
		if !pres {
			return errors.New("Unknown artifact " + name)
		}

		err := artifacts.Compile(artifact, vql_collector_args)
		if err != nil {
			return err
		}
	}

	// Add any artifact dependencies.
	repository.PopulateArtifactsVQLCollectorArgs(vql_collector_args)
	err = AddArtifactCollectorArgs(
		config_obj, vql_collector_args, collector_args)
	if err != nil {
		return err
	}

	return QueueMessageForClient(
		config_obj, flow_obj,
		"VQLClientAction",
		vql_collector_args, processVQLResponses)
}

func (self *ArtifactCollector) ProcessMessage(
	config_obj *api_proto.Config,
	flow_obj *AFF4FlowObject,
	message *crypto_proto.GrrMessage) error {

	err := flow_obj.FailIfError(config_obj, message)
	if err != nil {
		return err
	}

	payload := responder.ExtractGrrMessagePayload(message)
	if payload == nil {
		return nil
	}

	switch message.RequestId {
	case constants.TransferWellKnownFlowId:
		log_path := path.Join(
			"clients", flow_obj.RunnerArgs.ClientId,
			"uploads", path.Base(message.SessionId))

		return appendDataToFile(
			config_obj, flow_obj, log_path, message)

	case processVQLResponses:
		if flow_obj.IsRequestComplete(message) {
			return flow_obj.Complete(config_obj)
		}

		response, ok := payload.(*actions_proto.VQLResponse)
		if !ok {
			return nil
		}
		log_path := path.Join(
			"clients", flow_obj.RunnerArgs.ClientId,
			"artifacts", response.Query.Name,
			path.Base(flow_obj.Urn))

		// Store the event log in the client's VFS.
		if response.Query.Name != "" {
			file_store_factory := file_store.GetFileStore(config_obj)
			fd, err := file_store_factory.WriteFile(log_path + ".csv")
			if err != nil {
				fmt.Printf("Error: %v\n", err)
				return err
			}
			defer fd.Close()

			writer, err := csv.GetCSVWriter(vql_subsystem.MakeScope(), fd)
			if err != nil {
				fmt.Printf("Error: %v\n", err)
				return err
			}
			defer writer.Close()

			// Decode the JSON data.
			var rows []map[string]interface{}
			err = json.Unmarshal([]byte(response.Response), &rows)
			if err != nil {
				return errors.WithStack(err)
			}

			for _, row := range rows {
				csv_row := vfilter.NewDict()

				for _, column := range response.Columns {
					item, pres := row[column]
					if !pres {
						csv_row.Set(column, "-")
					} else {
						csv_row.Set(column, item)
					}
				}

				writer.Write(csv_row)
			}
		}
	}
	return nil
}

// Adds any parameters set in the ArtifactCollectorArgs into the
// VQLCollectorArgs.
func AddArtifactCollectorArgs(
	config_obj *api_proto.Config,
	vql_collector_args *actions_proto.VQLCollectorArgs,
	collector_args *flows_proto.ArtifactCollectorArgs) error {

	// Add any Environment Parameters from the request.
	if collector_args.Parameters == nil {
		return nil
	}

	for _, item := range collector_args.Parameters.Env {
		vql_collector_args.Env = append(vql_collector_args.Env,
			&actions_proto.VQLEnv{Key: item.Key, Value: item.Value})
	}

	// Add any exported files.
	file_store_factory := file_store.GetFileStore(config_obj)

	for _, item := range collector_args.Parameters.Files {
		file, err := file_store_factory.ReadFile(path.Join(
			"/exported_files", item.Value))
		if err != nil {
			logger := logging.GetLogger(config_obj, &logging.ToolComponent)
			logger.WithFields(
				logrus.Fields{
					"filename": item.Value,
					"error":    fmt.Sprintf("%v", err),
				}).Error("Unable to read VFS file")
			return err
		}
		buf, err := ioutil.ReadAll(file)
		if err != nil {
			continue
		}
		vql_collector_args.Env = append(vql_collector_args.Env,
			&actions_proto.VQLEnv{
				Key:   item.Key,
				Value: string(buf),
			})
	}
	return nil
}

func init() {
	impl := ArtifactCollector{&VQLCollector{}}
	default_args, _ := ptypes.MarshalAny(&flows_proto.ArtifactCollectorArgs{})
	desc := &flows_proto.FlowDescriptor{
		Name:         "ArtifactCollector",
		FriendlyName: "Artifact Collector",
		Category:     "Collectors",
		Doc:          "Collects multiple artifacts at once.",
		ArgsType:     "ArtifactCollectorArgs",
		DefaultArgs:  default_args,
	}

	RegisterImplementation(desc, &impl)
}
