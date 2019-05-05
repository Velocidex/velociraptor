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
	"strings"

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
	utils "www.velocidex.com/golang/velociraptor/utils"
	vql_subsystem "www.velocidex.com/golang/velociraptor/vql"
	"www.velocidex.com/golang/vfilter"
)

const (
	_                          = iota
	processVQLResponses uint64 = iota
)

type ArtifactCollector struct {
	*BaseFlow
}

func (self *ArtifactCollector) New() Flow {
	return &ArtifactCollector{&BaseFlow{}}
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
		Timeout:      collector_args.Timeout,
	}
	for _, name := range collector_args.Artifacts.Names {
		artifact, pres := repository.Get(name)
		if !pres {
			return errors.New("Unknown artifact " + name)
		}

		err := repository.Compile(artifact, vql_collector_args)
		if err != nil {
			return err
		}
	}

	// Add any artifact dependencies.
	err = repository.PopulateArtifactsVQLCollectorArgs(vql_collector_args)
	if err != nil {
		return err
	}

	err = AddArtifactCollectorArgs(
		config_obj, vql_collector_args, collector_args)
	if err != nil {
		return err
	}

	err = artifacts.Obfuscate(config_obj, vql_collector_args)
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

		// Restore strings from flow state.
		err := artifacts.Deobfuscate(config_obj, response)
		if err != nil {
			return err
		}

		log_path := CalculateArtifactResultPath(
			flow_obj.RunnerArgs.ClientId,
			response.Query.Name,
			flow_obj.Urn)

		// Store the event log in the client's VFS.
		if response.Query.Name != "" {
			file_store_factory := file_store.GetFileStore(config_obj)
			fd, err := file_store_factory.WriteFile(log_path)
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

			// Update the artifacts with results in the
			// context.
			if len(rows) > 0 && !utils.InString(
				&flow_obj.FlowContext.ArtifactsWithResults,
				response.Query.Name) {
				flow_obj.FlowContext.
					ArtifactsWithResults = append(
					flow_obj.FlowContext.ArtifactsWithResults,
					response.Query.Name)
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

func appendDataToFile(
	config_obj *api_proto.Config,
	flow_obj *AFF4FlowObject,
	base_urn string,
	message *crypto_proto.GrrMessage) error {
	payload := responder.ExtractGrrMessagePayload(message)
	if payload == nil {
		return nil
	}

	file_buffer, ok := payload.(*actions_proto.FileBuffer)
	if !ok {
		return nil
	}
	file_store_factory := file_store.GetFileStore(config_obj)
	file_path := path.Join(base_urn, file_buffer.Pathspec.Accessor,
		file_buffer.Pathspec.Path)
	fd, err := file_store_factory.WriteFile(file_path)
	if err != nil {
		fmt.Printf("Error: %v\n", err)
		return err
	}
	defer fd.Close()

	fd.Seek(int64(file_buffer.Offset), 0)
	fd.Write(file_buffer.Data)

	// Keep track of all the files we uploaded.
	if file_buffer.Offset == 0 {
		flow_obj.FlowContext.UploadedFiles = append(
			flow_obj.FlowContext.UploadedFiles,
			file_path)
		flow_obj.dirty = true
	}
	return nil
}

// Figure out where the artifact result should be stored in the file
// store.

// NOTE: An artifact may have multiple sources and therefore contain
// multiple tables. However, each table is stored in its own CSV
// file. We therefore use a directory structure on the server to
// contain all sources related to the artifact:

// clients/<client_id>/artifacts/<artifact name>/<flow_id>/<source name>.csv

func CalculateArtifactResultPath(client_id, name, flow_urn string) string {

	// The artifact name is prepared by the artifact compiler. If
	// an artifact contains multiple sources, the query name will
	// consists of <artifact name>/<source name>.

	// This code places the source name under the artifact's main
	// result.
	components := strings.Split(name, "/")
	switch len(components) {
	case 2:
		source_name := components[1]
		artifact_name := components[0]
		return path.Join(
			"clients", client_id,
			"artifacts", artifact_name,
			path.Base(flow_urn), source_name+".csv")
	default:
		return path.Join(
			"clients", client_id,
			"artifacts", name,
			path.Base(flow_urn)+".csv")
	}
}

// Expand all the artifacts with their sources. Since each artifact
// can have multiple named sources, we return a list of name/source
// name type.
func ExpandArtifactNamesWithSouces(repository *artifacts.Repository,
	artifact_names []string) []string {
	result := []string{}

	for _, artifact_name := range artifact_names {
		names := []string{}
		artifact, pres := repository.Get(artifact_name)
		if !pres {
			continue
		}
		for _, source := range artifact.Sources {
			if source.Name != "" {
				names = append(
					names, artifact_name+"/"+source.Name)
			}
		}

		if names == nil {
			result = append(result, artifact_name)
		} else {
			result = append(result, names...)
		}
	}

	return result
}

func init() {
	impl := ArtifactCollector{&BaseFlow{}}
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
