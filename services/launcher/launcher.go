/*
  Launches new collection against clients.
*/

package launcher

import (
	"context"
	"crypto/rand"
	"encoding/base32"
	"encoding/binary"
	"fmt"
	"sync"
	"time"

	errors "github.com/pkg/errors"
	"google.golang.org/protobuf/proto"
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
	"www.velocidex.com/golang/velociraptor/services"
	vql_subsystem "www.velocidex.com/golang/velociraptor/vql"
)

// Ensures the specs field corresponds exactly with the
// collector_request.Artifacts field: Extra fields are removed and
// missing fields are added.
func getCollectorSpecs(
	collector_request *flows_proto.ArtifactCollectorArgs) []*flows_proto.ArtifactSpec {

	// Find the spec in the collector_request.Specs list.
	get_spec := func(name string) *flows_proto.ArtifactSpec {
		for _, spec := range collector_request.Specs {
			if name == spec.Artifact {
				return spec
			}
		}

		return nil
	}

	result := []*flows_proto.ArtifactSpec{}

	// Find all the specs from the artifacts list.
	for _, name := range collector_request.Artifacts {
		spec := get_spec(name)
		if spec == nil {
			spec = &flows_proto.ArtifactSpec{
				Artifact: name,
			}
		}

		result = append(result, spec)
	}

	return result
}

type Launcher struct{}

func (self *Launcher) CompileCollectorArgs(
	ctx context.Context,
	config_obj *config_proto.Config,
	acl_manager vql_subsystem.ACLManager,
	repository services.Repository,
	should_obfuscate bool,
	collector_request *flows_proto.ArtifactCollectorArgs) (
	[]*actions_proto.VQLCollectorArgs, error) {

	result := []*actions_proto.VQLCollectorArgs{}

	for _, spec := range getCollectorSpecs(collector_request) {
		var artifact *artifacts_proto.Artifact = nil

		if collector_request.AllowCustomOverrides {
			artifact, _ = repository.Get(config_obj, "Custom."+spec.Artifact)
		}

		if artifact == nil {
			artifact, _ = repository.Get(config_obj, spec.Artifact)
		}

		if artifact == nil {
			return nil, errors.New("Unknown artifact " + spec.Artifact)
		}

		err := CheckAccess(config_obj, artifact, acl_manager)
		if err != nil {
			return nil, err
		}

		for _, expanded_artifact := range expandArtifacts(artifact) {
			vql_collector_args, err := self.getVQLCollectorArgs(
				ctx, config_obj, repository, expanded_artifact,
				spec, should_obfuscate)
			if err != nil {
				return nil, err
			}

			vql_collector_args.OpsPerSecond = collector_request.OpsPerSecond
			vql_collector_args.Timeout = collector_request.Timeout
			vql_collector_args.MaxRow = 1000

			result = append(result, vql_collector_args)
		}
	}

	return result, nil
}

// Normally each artifact is collected in order - the first source,
// then the second source etc. However, for event artifacts, this is
// not possible because each source never terminates. Therefore for
// event artifacts, we expand the artifact with multiple sources, into
// multiple artifacts with one source each in order to ensure each
// source is collected independently.
func expandArtifacts(artifact *artifacts_proto.Artifact) []*artifacts_proto.Artifact {
	switch artifact.Type {
	default:
		return []*artifacts_proto.Artifact{artifact}

	case "server_event", "client_event":
		result := []*artifacts_proto.Artifact{}
		for _, source := range artifact.Sources {
			new_artifact := proto.Clone(artifact).(*artifacts_proto.Artifact)
			new_artifact.Sources = []*artifacts_proto.ArtifactSource{source}
			result = append(result, new_artifact)
		}
		return result
	}
}

// Compile a single artifact, resolve dependencies and tools
func (self *Launcher) getVQLCollectorArgs(
	ctx context.Context,
	config_obj *config_proto.Config,
	repository services.Repository,
	artifact *artifacts_proto.Artifact,
	spec *flows_proto.ArtifactSpec,
	should_obfuscate bool) (*actions_proto.VQLCollectorArgs, error) {

	vql_collector_args := &actions_proto.VQLCollectorArgs{}
	err := CompileSingleArtifact(config_obj, repository, artifact, vql_collector_args)
	if err != nil {
		return nil, err
	}

	err = self.EnsureToolsDeclared(ctx, config_obj, artifact)
	if err != nil {
		return nil, err
	}

	// Add any artifact dependencies.
	err = PopulateArtifactsVQLCollectorArgs(
		config_obj, repository, vql_collector_args)
	if err != nil {
		return nil, err
	}

	err = self.AddArtifactCollectorArgs(vql_collector_args, spec)
	if err != nil {
		return nil, err
	}

	err = getDependentTools(ctx, config_obj, vql_collector_args)
	if err != nil {
		return nil, err
	}

	if should_obfuscate {
		err = artifacts.Obfuscate(config_obj, vql_collector_args)
		if err != nil {
			return nil, err
		}
	}
	return vql_collector_args, nil
}

// Make sure we know about tools the artifact itself defines.
func (self *Launcher) EnsureToolsDeclared(
	ctx context.Context,
	config_obj *config_proto.Config,
	artifact *artifacts_proto.Artifact) error {

	logger := logging.GetLogger(config_obj, &logging.FrontendComponent)
	for _, tool := range artifact.Tools {
		_, err := services.GetInventory().GetToolInfo(ctx, config_obj, tool.Name)
		if err != nil {
			// Add tool info if it is not known but do not
			// override existing tool. This allows the
			// admin to override tools from the artifact
			// itself.
			logger.Info("Adding tool %v from artifact %v",
				tool.Name, artifact.Name)
			err = services.GetInventory().AddTool(
				config_obj, tool,
				services.ToolOptions{
					Upgrade: true,
				})
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
	inventory := services.GetInventory()
	if inventory == nil {
		return errors.New("Inventory server not configured")
	}

	tool_info, err := inventory.GetToolInfo(ctx, config_obj, tool)
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

	// Support local filesystem access for local tools.
	if tool_info.ServePath != "" {
		vql_collector_args.Env = append(vql_collector_args.Env, &actions_proto.VQLEnv{
			Key:   fmt.Sprintf("Tool_%v_PATH", tool_info.Name),
			Value: tool_info.ServePath,
		})
	} else if tool_info.ServeUrl != "" {
		// Where to download the binary from.
		url := ""

		// If we dont want to serve the binary locally, just
		// tell the client where to get it from.
		if tool_info.ServeUrl != "" {
			url = tool_info.ServeUrl

		} else if tool_info.Url != "" {
			url = tool_info.Url

		} else if config_obj.Client != nil {
			url = config_obj.Client.ServerUrls[0] + "public/" + tool_info.FilestorePath
		}

		vql_collector_args.Env = append(vql_collector_args.Env, &actions_proto.VQLEnv{
			Key:   fmt.Sprintf("Tool_%v_URL", tool_info.Name),
			Value: url,
		})
	}
	return nil
}

func (self *Launcher) ScheduleArtifactCollection(
	ctx context.Context,
	config_obj *config_proto.Config,
	acl_manager vql_subsystem.ACLManager,
	repository services.Repository,
	collector_request *flows_proto.ArtifactCollectorArgs) (string, error) {

	args := collector_request.CompiledCollectorArgs
	if args == nil {
		// Compile and cache the compilation for next time
		// just in case this request is reused.

		// NOTE: We assume that compiling the artifact is a
		// pure function so caching is appropriate.
		compiled, err := self.CompileCollectorArgs(
			ctx, config_obj, acl_manager, repository,
			true, /* should_obfuscate */
			collector_request)
		if err != nil {
			return "", err
		}
		args = append(args, compiled...)
	}

	return ScheduleArtifactCollectionFromCollectorArgs(
		config_obj, collector_request, args)
}

func ScheduleArtifactCollectionFromCollectorArgs(
	config_obj *config_proto.Config,
	collector_request *flows_proto.ArtifactCollectorArgs,
	vql_collector_args []*actions_proto.VQLCollectorArgs) (string, error) {

	client_id := collector_request.ClientId
	if client_id == "" {
		return "", errors.New("Client id not provided.")
	}

	// Generate a new collection context.
	collection_context := &flows_proto.ArtifactCollectorContext{
		SessionId:           NewFlowId(client_id),
		CreateTime:          uint64(time.Now().UnixNano() / 1000),
		State:               flows_proto.ArtifactCollectorContext_RUNNING,
		Request:             collector_request,
		ClientId:            client_id,
		OutstandingRequests: int64(len(vql_collector_args)),
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

	tasks := []*crypto_proto.GrrMessage{}

	for _, arg := range vql_collector_args {
		// If sending to the server record who actually launched this.
		if client_id == "server" {
			arg.Principal = collection_context.Request.Creator
		}

		// The task we will schedule for the client.
		task := &crypto_proto.GrrMessage{
			SessionId:       collection_context.SessionId,
			RequestId:       constants.ProcessVQLResponses,
			VQLClientAction: arg,
		}

		// Send an urgent request to the client.
		if collector_request.Urgent {
			task.Urgent = true
		}

		err = db.QueueMessageForClient(
			config_obj, client_id, task)
		if err != nil {
			return "", err
		}
		tasks = append(tasks, task)
	}

	// Record the tasks for provenance of what we actually did.
	err = db.SetSubject(config_obj,
		flow_path_manager.Task().Path(),
		&api_proto.ApiFlowRequestDetails{Items: tasks})
	if err != nil {
		return "", err
	}

	return collection_context.SessionId, nil
}

// Adds any parameters set in the ArtifactCollectorArgs into the
// VQLCollectorArgs.
func (self *Launcher) AddArtifactCollectorArgs(
	vql_collector_args *actions_proto.VQLCollectorArgs,
	spec *flows_proto.ArtifactSpec) error {

	// Add any Environment Parameters from the request.
	if spec.Parameters == nil {
		return nil
	}

	// We can only specify a parameter which is defined already
	if spec.Parameters != nil {
		for _, item := range spec.Parameters.Env {
			vql_collector_args.Env = addOrReplaceParameter(
				item, vql_collector_args.Env)
		}
	}

	return nil
}

// We do not expect too many parameters so linear search is appropriate.
func addOrReplaceParameter(
	param *actions_proto.VQLEnv, env []*actions_proto.VQLEnv) []*actions_proto.VQLEnv {
	result := append([]*actions_proto.VQLEnv(nil), env...)

	// Try to replace it if it is already there.
	for _, item := range result {
		if item.Key == param.Key {
			item.Value = param.Value
			return result
		}
	}
	return append(result, param)
}

func (self *Launcher) SetFlowIdForTests(id string) {
	NextFlowIdForTests = id
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
	_, _ = rand.Read(buf)

	binary.BigEndian.PutUint32(buf, uint32(time.Now().Unix()))
	result := base32.HexEncoding.EncodeToString(buf)[:13]

	return constants.FLOW_PREFIX + result
}

func StartLauncherService(
	ctx context.Context,
	wg *sync.WaitGroup,
	config_obj *config_proto.Config) error {

	wg.Add(1)
	go func() {
		defer wg.Done()
		defer services.RegisterLauncher(nil)

		<-ctx.Done()
	}()

	services.RegisterLauncher(&Launcher{})
	return nil
}
