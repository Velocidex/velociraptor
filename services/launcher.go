/*
  Launches new collection against clients.
*/

package services

import (
	"context"
	"crypto/rand"
	"encoding/base32"
	"encoding/binary"
	"fmt"
	"time"

	errors "github.com/pkg/errors"
	actions_proto "www.velocidex.com/golang/velociraptor/actions/proto"
	api_proto "www.velocidex.com/golang/velociraptor/api/proto"
	"www.velocidex.com/golang/velociraptor/artifacts"
	artifacts_proto "www.velocidex.com/golang/velociraptor/artifacts/proto"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	"www.velocidex.com/golang/velociraptor/constants"
	crypto_proto "www.velocidex.com/golang/velociraptor/crypto/proto"
	"www.velocidex.com/golang/velociraptor/datastore"
	flows_proto "www.velocidex.com/golang/velociraptor/flows/proto"
	"www.velocidex.com/golang/velociraptor/logging"
	"www.velocidex.com/golang/velociraptor/paths"
)

func CompileCollectorArgs(
	ctx context.Context,
	config_obj *config_proto.Config,
	principal string,
	collector_request *flows_proto.ArtifactCollectorArgs) (
	*actions_proto.VQLCollectorArgs, error) {
	repository, err := artifacts.GetGlobalRepository(config_obj)
	if err != nil {
		return nil, err
	}

	// Update the flow's artifacts list.
	vql_collector_args := &actions_proto.VQLCollectorArgs{
		OpsPerSecond: collector_request.OpsPerSecond,
		Timeout:      collector_request.Timeout,
		MaxRow:       1000,
	}
	for _, name := range collector_request.Artifacts {
		var artifact *artifacts_proto.Artifact = nil
		if collector_request.AllowCustomOverrides {
			artifact, _ = repository.Get("Custom." + name)
		}

		if artifact == nil {
			artifact, _ = repository.Get(name)
		}

		if artifact == nil {
			return nil, errors.New("Unknown artifact " + name)
		}

		err := repository.CheckAccess(config_obj, artifact, principal)
		if err != nil {
			return nil, err
		}

		err = repository.Compile(artifact, vql_collector_args)
		if err != nil {
			return nil, err
		}

		ensureToolsDeclared(ctx, config_obj, artifact)
	}

	// Add any artifact dependencies.
	err = repository.PopulateArtifactsVQLCollectorArgs(vql_collector_args)
	if err != nil {
		return nil, err
	}

	err = AddArtifactCollectorArgs(
		config_obj, vql_collector_args, collector_request)
	if err != nil {
		return nil, err
	}

	err = getDependentTools(ctx, config_obj, vql_collector_args)
	if err != nil {
		return nil, err
	}

	err = artifacts.Obfuscate(config_obj, vql_collector_args)
	return vql_collector_args, err
}

func getDependentTools(
	ctx context.Context,
	config_obj *config_proto.Config,
	vql_collector_args *actions_proto.VQLCollectorArgs) error {

	logger := logging.GetLogger(config_obj, &logging.FrontendComponent)
	for _, tool := range vql_collector_args.Tools {
		err := AddToolDependency(ctx, config_obj, tool, vql_collector_args)
		if err != nil {
			logger.Error("While Adding dependencies: %v", err)
			return err
		}
	}

	return nil
}

// Make sure we know about tools the artifact itself defines.
func ensureToolsDeclared(
	ctx context.Context, config_obj *config_proto.Config,
	artifact *artifacts_proto.Artifact) error {

	logger := logging.GetLogger(config_obj, &logging.FrontendComponent)
	for _, tool := range artifact.Tools {
		_, err := Inventory.GetToolInfo(ctx, config_obj, tool.Name)
		if err != nil {
			// Add tool info if it is not known but do not
			// override existing tool. This allows the
			// admin to override tools from the artifact
			// itself.
			logger.Info("Adding tool %v from artifact %v",
				tool.Name, artifact.Name)
			err = Inventory.AddTool(config_obj, tool)
			if err != nil {
				return err
			}
		}
	}
	return nil
}

func AddToolDependency(
	ctx context.Context,
	config_obj *config_proto.Config,
	tool string, vql_collector_args *actions_proto.VQLCollectorArgs) error {
	tool_info, err := Inventory.GetToolInfo(ctx, config_obj, tool)
	if err != nil {
		return err
	}

	vql_collector_args.Env = append(vql_collector_args.Env, &actions_proto.VQLEnv{
		Key:   fmt.Sprintf("Tool_%v_HASH", tool_info.Name),
		Value: tool_info.Hash,
	})

	vql_collector_args.Env = append(vql_collector_args.Env, &actions_proto.VQLEnv{
		Key:   fmt.Sprintf("Tool_%v_FILENAME", tool_info.Name),
		Value: tool_info.Filename,
	})

	if len(config_obj.Client.ServerUrls) == 0 {
		return errors.New("No server URLs configured!")
	}

	// Where to download the binary from.
	url := config_obj.Client.ServerUrls[0] + "public/" + tool_info.FilestorePath

	// If we dont want to serve the binary locally, just
	// tell the client where to get it from.
	if !tool_info.ServeLocally && tool_info.Url != "" {
		url = tool_info.Url
	}
	vql_collector_args.Env = append(vql_collector_args.Env, &actions_proto.VQLEnv{
		Key:   fmt.Sprintf("Tool_%v_URL", tool_info.Name),
		Value: url,
	})
	return nil
}

func ScheduleArtifactCollection(
	ctx context.Context,
	config_obj *config_proto.Config,
	principal string,
	collector_request *flows_proto.ArtifactCollectorArgs) (string, error) {

	args := collector_request.CompiledCollectorArgs
	if args == nil {
		var err error
		args, err = CompileCollectorArgs(
			ctx, config_obj, principal, collector_request)
		if err != nil {
			return "", err
		}
	}

	return ScheduleArtifactCollectionFromCollectorArgs(
		config_obj, collector_request, args)
}

func ScheduleArtifactCollectionFromCollectorArgs(
	config_obj *config_proto.Config,
	collector_request *flows_proto.ArtifactCollectorArgs,
	vql_collector_args *actions_proto.VQLCollectorArgs) (string, error) {

	client_id := collector_request.ClientId
	if client_id == "" {
		return "", errors.New("Client id not provided.")
	}

	// Generate a new collection context.
	collection_context := &flows_proto.ArtifactCollectorContext{
		SessionId:  NewFlowId(client_id),
		CreateTime: uint64(time.Now().UnixNano() / 1000),
		State:      flows_proto.ArtifactCollectorContext_RUNNING,
		Request:    collector_request,
		ClientId:   client_id,
	}
	db, err := datastore.GetDB(config_obj)
	if err != nil {
		return "", err
	}

	// Save the collection context.
	flow_path_manager := paths.NewFlowPathManager(client_id,
		collection_context.SessionId)
	err = db.SetSubject(config_obj,
		flow_path_manager.Path(),
		collection_context)
	if err != nil {
		return "", err
	}

	// The task we will schedule for the client.
	task := &crypto_proto.GrrMessage{
		SessionId:       collection_context.SessionId,
		RequestId:       constants.ProcessVQLResponses,
		VQLClientAction: vql_collector_args}

	// Send an urgent request to the client.
	if collector_request.Urgent {
		task.Urgent = true
	}

	// Record the tasks for provenance of what we actually did.
	err = db.SetSubject(config_obj,
		flow_path_manager.Task().Path(),
		&api_proto.ApiFlowRequestDetails{
			Items: []*crypto_proto.GrrMessage{task}})
	if err != nil {
		return "", err
	}

	return collection_context.SessionId, db.QueueMessageForClient(
		config_obj, client_id, task)
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

	return nil
}

var (
	NextFlowIdForTests string
)

func NewFlowId(client_id string) string {
	if NextFlowIdForTests != "" {
		result := NextFlowIdForTests
		NextFlowIdForTests = ""
		return result
	}

	buf := make([]byte, 8)
	rand.Read(buf)

	binary.BigEndian.PutUint32(buf, uint32(time.Now().Unix()))
	result := base32.HexEncoding.EncodeToString(buf)[:13]

	return constants.FLOW_PREFIX + result
}
