package artifacts

import (
	"crypto/rand"
	"encoding/base32"
	"encoding/binary"
	"time"

	errors "github.com/pkg/errors"
	actions_proto "www.velocidex.com/golang/velociraptor/actions/proto"
	api_proto "www.velocidex.com/golang/velociraptor/api/proto"
	artifacts_proto "www.velocidex.com/golang/velociraptor/artifacts/proto"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	"www.velocidex.com/golang/velociraptor/constants"
	crypto_proto "www.velocidex.com/golang/velociraptor/crypto/proto"
	"www.velocidex.com/golang/velociraptor/datastore"
	flows_proto "www.velocidex.com/golang/velociraptor/flows/proto"
	"www.velocidex.com/golang/velociraptor/paths"
)

func CompileCollectorArgs(
	config_obj *config_proto.Config,
	principal string,
	collector_request *flows_proto.ArtifactCollectorArgs) (
	*actions_proto.VQLCollectorArgs, error) {
	repository, err := GetGlobalRepository(config_obj)
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

	err = Obfuscate(config_obj, vql_collector_args)
	return vql_collector_args, err
}

func ScheduleArtifactCollection(
	config_obj *config_proto.Config,
	principal string,
	collector_request *flows_proto.ArtifactCollectorArgs) (string, error) {

	args := collector_request.CompiledCollectorArgs
	if args == nil {
		var err error
		args, err = CompileCollectorArgs(
			config_obj, principal, collector_request)
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
