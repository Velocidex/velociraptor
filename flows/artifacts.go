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
	"io"
	"io/ioutil"
	"path"
	"time"

	"github.com/golang/protobuf/proto"
	"github.com/golang/protobuf/ptypes"
	errors "github.com/pkg/errors"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"github.com/sirupsen/logrus"
	actions_proto "www.velocidex.com/golang/velociraptor/actions/proto"
	api_proto "www.velocidex.com/golang/velociraptor/api/proto"
	artifacts "www.velocidex.com/golang/velociraptor/artifacts"
	artifacts_proto "www.velocidex.com/golang/velociraptor/artifacts/proto"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	constants "www.velocidex.com/golang/velociraptor/constants"
	crypto_proto "www.velocidex.com/golang/velociraptor/crypto/proto"
	"www.velocidex.com/golang/velociraptor/file_store"
	"www.velocidex.com/golang/velociraptor/file_store/csv"
	flows_proto "www.velocidex.com/golang/velociraptor/flows/proto"
	"www.velocidex.com/golang/velociraptor/logging"
	"www.velocidex.com/golang/velociraptor/services"
	utils "www.velocidex.com/golang/velociraptor/utils"
	vql_subsystem "www.velocidex.com/golang/velociraptor/vql"
	"www.velocidex.com/golang/vfilter"
)

var (
	uploadCounter = promauto.NewCounter(prometheus.CounterOpts{
		Name: "uploaded_files",
		Help: "Total number of Uploaded Files.",
	})
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
	config_obj *config_proto.Config,
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
	flow_obj.SetContext(flow_obj.FlowContext)

	vql_collector_args := &actions_proto.VQLCollectorArgs{
		OpsPerSecond: collector_args.OpsPerSecond,
		Timeout:      collector_args.Timeout,
	}
	for _, name := range collector_args.Artifacts.Names {
		var artifact *artifacts_proto.Artifact = nil
		if collector_args.AllowCustomOverrides {
			artifact, _ = repository.Get("Custom." + name)
		}

		if artifact == nil {
			artifact, _ = repository.Get(name)
		}

		if artifact == nil {
			return errors.New("Unknown artifact " + name)
		}

		err := repository.Compile(artifact, vql_collector_args)
		if err != nil {
			return err
		}

		flow_obj.FlowContext.Artifacts = append(flow_obj.FlowContext.Artifacts,
			artifact.Name)
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
		config_obj, flow_obj.RunnerArgs.ClientId,
		&crypto_proto.GrrMessage{
			SessionId:       flow_obj.Urn,
			RequestId:       processVQLResponses,
			VQLClientAction: vql_collector_args})
}

func (self *ArtifactCollector) ProcessMessage(
	config_obj *config_proto.Config,
	flow_obj *AFF4FlowObject,
	message *crypto_proto.GrrMessage) error {

	err := flow_obj.FailIfError(config_obj, message)
	if err != nil {
		if constants.HuntIdRegex.MatchString(flow_obj.RunnerArgs.Creator) {
			err = services.GetHuntDispatcher().ModifyHunt(
				flow_obj.RunnerArgs.Creator,
				func(hunt *api_proto.Hunt) error {
					hunt.Stats.TotalClientsWithErrors++
					return nil
				})
		}

		return err
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
			if constants.HuntIdRegex.MatchString(flow_obj.RunnerArgs.Creator) {
				err = services.GetHuntDispatcher().ModifyHunt(
					flow_obj.RunnerArgs.Creator,
					func(hunt *api_proto.Hunt) error {
						hunt.Stats.TotalClientsWithResults++
						return nil
					})
				if err != nil {
					return err
				}
			}

			return flow_obj.Complete(config_obj)
		}

		// Restore strings from flow state.
		response := message.VQLResponse
		if response == nil {
			return errors.New("Expected args of type VQLResponse")
		}

		err := artifacts.Deobfuscate(config_obj, response)
		if err != nil {
			return err
		}

		artifact_name, source_name := artifacts.
			QueryNameToArtifactAndSource(response.Query.Name)

		// Store the event log in the client's VFS.
		if response.Query.Name != "" {
			log_path := artifacts.GetCSVPath(
				flow_obj.RunnerArgs.ClientId, "",
				path.Base(flow_obj.Urn),
				artifact_name, source_name, artifacts.MODE_CLIENT)

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
	config_obj *config_proto.Config,
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
		defer file.Close()

		buf, err := ioutil.ReadAll(
			io.LimitReader(file, constants.MAX_MEMORY))
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
	config_obj *config_proto.Config,
	flow_obj *AFF4FlowObject,
	base_urn string,
	message *crypto_proto.GrrMessage) error {
	file_buffer := message.FileBuffer
	if file_buffer == nil {
		return errors.New("Expected args of type FileBuffer")
	}

	file_store_factory := file_store.GetFileStore(config_obj)
	file_path := path.Join(base_urn, file_buffer.Pathspec.Accessor,
		file_buffer.Pathspec.Path)
	fd, err := file_store_factory.WriteFile(file_path)
	if err != nil {
		// If we fail to write this one file we keep going -
		// otherwise the flow will be terminated.
		flow_obj.Log(config_obj, fmt.Sprintf("While writing to %v: %v",
			file_path, err))
		return nil
	}
	defer fd.Close()

	// Keep track of all the files we uploaded.
	if file_buffer.Offset == 0 {
		fd.Truncate(0)
		flow_obj.FlowContext.TotalUploadedFiles += 1
		flow_obj.FlowContext.TotalExpectedUploadedBytes += file_buffer.Size
		flow_obj.FlowContext.UploadedFiles = append(
			flow_obj.FlowContext.UploadedFiles,
			&flows_proto.UploadedFileInfo{
				Name: file_path,
				Size: file_buffer.Size,
			})
		flow_obj.dirty = true
	}

	if len(file_buffer.Data) > 0 {
		flow_obj.FlowContext.TotalUploadedBytes += uint64(len(file_buffer.Data))
		flow_obj.dirty = true
	}

	fd.Seek(int64(file_buffer.Offset), 0)
	_, err = fd.Write(file_buffer.Data)
	if err != nil {
		flow_obj.Log(config_obj, fmt.Sprintf("While writing to %v: %v",
			file_path, err))
		return nil
	}

	// When the upload completes, we emit an event.
	if file_buffer.Eof {
		uploadCounter.Inc()

		row := vfilter.NewDict().
			Set("Timestamp", time.Now().UTC().Unix()).
			Set("ClientId", flow_obj.RunnerArgs.ClientId).
			Set("VFSPath", file_path).
			Set("UploadName", file_buffer.Pathspec.Path).
			Set("Accessor", file_buffer.Pathspec.Accessor).
			Set("Size", file_buffer.Offset+uint64(
				len(file_buffer.Data)))

		serialized, err := json.Marshal([]vfilter.Row{row})
		if err == nil {
			gJournalWriter.Channel <- &Event{
				Config:    config_obj,
				ClientId:  flow_obj.RunnerArgs.ClientId,
				QueryName: "System.Upload.Completion",
				Response:  string(serialized),
				Columns: []string{"Timestamp", "ClientId",
					"VFSPath", "UploadName",
					"Accessor", "Size"},
			}
		}
	}

	return nil
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
