/*
  Launches new collection against clients.

  Artifacts are YAML files which encapsultate VQL queries in human
  readable contextual package. The launcher service is responsible for
  compiling artifacts into direct client requests. Clients run direct
  VQL statements derived from the artifacts, while users write,
  customize, or launch artifacts.

  Compiling the artifact into client requests consists of:

  1. Splitting artifact sources into separate requests, each running
     independently.
  2. Type conversion of artifact parameters into the VQL scope prior
     to artifact VQL code execution.

  Compiling an artifact is currently a pure function, this means it is
  possible to cache the compiled artifact in the artifact repository
  for future use.

  ## Sources

  An artifact may contains several sources. Each source represents a
  single SELECT query and potentially multiple LET queries. Ultimately
  each source returns a single table of results. If an artifact wishes
  to return multiple tables, it should define multiple sources.

  It is sometimes useful to run multiple sources in the same
  scope. This allows for example a result set to be calculated in the
  first source, then presented in the second source, or be the basis
  of further calculation in the third source. Thereby returning a
  series of related tables. We call this mode of execution "Serial
  Mode" since in this mode, source1 will be collected, then source 2
  etc in the same client request.

  Similarly for event plugins, it is impossible to run in serial mode
  because each source never terminates. Therefore event artifacts
  (SERVER_EVENT, CLIENT_EVENT) produce multiple independent requests
  from the client. We call this "Parallel mode" as each request is
  independent and runs in parallel.

  The most important distinction from the artifacts writer's POV is
  that serial mode reuses the scope between sources, while parallel
  mode uses a new scope for each source.

  ```yaml
  name: MultiSourceSerialMode
  sources:
  - name: Source1
    query: |
      LET X <= SELECT ....
      SELECT ...
  - name: Source2
    query: |
      SELECT * FROM X
  ```

  Consider the above artifact which will run serially - First Source1
  and then Source2 in the same request. Therefore Source2 can see any
  queries or results defined in Source1.

  ## Preconditions

  A precondition is a query that will run before the main
  collection. If the precondition returns any rows then it is deemed
  to be TRUE and therefore the main query will be run. Otherwise, the
  request will be ignored by the client. Preconditions allow one to
  control execution of the artifact so it is safe to collect it on a
  wider group of systems (e.g. Linux only artifacts may safely collect
  on windows but will do nothing at all).

  Artifacts have two places where preconditions may be
  defined. Preconditions may be defined at the top level, in which
  case they apply to all sources. However preconditions may also be
  defined on each source, in this case the source will not be
  collected unless the precondition is true.

  Consider the following artifact:

  ```yaml
  name: MultiSourceSerialMode
  sources:
  - name: Source1
    precondition: SELECT * FROM info() WHERE OS = "linux"
    query: |
      LET X <= SELECT ....
      SELECT ...
  - name: Source2
    precondition: SELECT * FROM info() WHERE OS = "windows"
    query: |
      SELECT * FROM X
  ```

  Source1 will only run on Linux systems, and Source2 on Windows
  systems. Therefore it is impossible to share scope between the two
  sources since Source2 can never see the variable X defined by
  Source1.

  Therefore when preconditions are defined at the source level, the
  artifact will be collected in "Parallel Mode", implying each source
  has its own scope.

  ## Summary

  The following rules summarise if the artifact is collected in
  parallel mode (i.e. sources in separate requests) or Serial Mode
  (i.e. all sources in the same request).

  * Event artifacts:                  Parallel Mode
  * No preconditions:                 Serial Mode
  * Precondition at the top level:    Serial Mode
  * Precondition at source level:     Parallel Mode

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
	options services.CompilerOptions,
	collector_request *flows_proto.ArtifactCollectorArgs) (
	[]*actions_proto.VQLCollectorArgs, error) {

	result := []*actions_proto.VQLCollectorArgs{}

	// We extract the default resource limits from each artifact
	// definition and calculate a collection wide default. For
	// example if a collection specifies artifact A (with max_rows
	// = 10) and artifact B (with max_rows = 20), then the
	// collection will have max_rows = 20.
	var max_rows, max_upload_bytes, timeout uint64
	var ops_per_sec, cpu_limit, iops_limit float32

	for _, spec := range getCollectorSpecs(collector_request) {
		var artifact *artifacts_proto.Artifact = nil

		if collector_request.AllowCustomOverrides {
			artifact, _ = repository.Get(config_obj, "Custom."+spec.Artifact)
		}

		if artifact == nil {
			artifact, _ = repository.Get(config_obj, spec.Artifact)
		}

		if artifact == nil {
			// We have not found the artifact, should we ignore the error?
			if options.IgnoreMissingArtifacts {
				logger := logging.GetLogger(config_obj, &logging.FrontendComponent)
				logger.Info("Ignoring artifact %v since it is not found",
					spec.Artifact)
				continue
			}
			return nil, errors.New("Unknown artifact " + spec.Artifact)
		}

		err := CheckAccess(config_obj, artifact, acl_manager)
		if err != nil {
			return nil, err
		}

		// Adjust collection wide resources to be the maximum
		// number of all default
		if artifact.Resources != nil {
			if artifact.Resources.MaxRows > max_rows {
				max_rows = artifact.Resources.MaxRows
			}

			if artifact.Resources.MaxUploadBytes > max_upload_bytes {
				max_upload_bytes = artifact.Resources.MaxUploadBytes
			}
		}

		for _, expanded_artifact := range expandArtifacts(artifact) {
			vql_collector_args, err := self.GetVQLCollectorArgs(
				ctx, config_obj, repository, expanded_artifact,
				spec, options)
			if err != nil {
				return nil, err
			}

			// If the request specifies resource controls
			// they override the defaults.
			if collector_request.OpsPerSecond > 0 {
				vql_collector_args.OpsPerSecond = collector_request.OpsPerSecond
			}

			if collector_request.CpuLimit > 0 {
				vql_collector_args.CpuLimit = collector_request.CpuLimit
			}

			if collector_request.IopsLimit > 0 {
				vql_collector_args.IopsLimit = collector_request.IopsLimit
			}

			if collector_request.Timeout > 0 {
				vql_collector_args.Timeout = collector_request.Timeout
			}

			vql_collector_args.MaxRow = 1000

			timeout = vql_collector_args.Timeout
			ops_per_sec = vql_collector_args.OpsPerSecond
			cpu_limit = vql_collector_args.CpuLimit
			iops_limit = vql_collector_args.IopsLimit

			result = append(result, vql_collector_args)
		}
	}

	// Adjust the collection wide resources to take the defaults
	// from artifacts definitions.
	if collector_request.MaxRows == 0 {
		collector_request.MaxRows = max_rows
	}

	if collector_request.MaxUploadBytes == 0 {
		collector_request.MaxUploadBytes = max_upload_bytes
	}

	if collector_request.Timeout == 0 {
		collector_request.Timeout = timeout
	}

	if collector_request.OpsPerSecond == 0 {
		collector_request.OpsPerSecond = ops_per_sec
	}

	if collector_request.CpuLimit == 0 {
		collector_request.CpuLimit = cpu_limit
	}

	if collector_request.IopsLimit == 0 {
		collector_request.IopsLimit = iops_limit
	}

	return result, nil
}

// expandArtifacts converts a user artifact with multiple sources into
// equivalent single source artifacts. Conversion occurs according to
// the rules at the top of this file. Each single source artifact will
// be converted to a single client request.
func expandArtifacts(artifact *artifacts_proto.Artifact) []*artifacts_proto.Artifact {
	if artifact.Type == "server_event" || artifact.Type == "client_event" {
		result := []*artifacts_proto.Artifact{}
		for _, source := range artifact.Sources {
			new_artifact := proto.Clone(artifact).(*artifacts_proto.Artifact)
			new_artifact.Sources = []*artifacts_proto.ArtifactSource{source}
			// A precondition at the source level will
			// override an artifact wide preconditon.
			if source.Precondition != "" {
				new_artifact.Precondition = source.Precondition
			}
			new_artifact.Resources = artifact.Resources

			result = append(result, new_artifact)
		}
		return result
	}

	if artifact.Precondition != "" {
		return []*artifacts_proto.Artifact{artifact}
	}

	has_source_precondition := false
	for _, source := range artifact.Sources {
		if source.Precondition != "" {
			has_source_precondition = true
			break
		}
	}

	if !has_source_precondition {
		return []*artifacts_proto.Artifact{artifact}
	}

	// Artifact has source preconditions, we duplicate the
	// artifact and copy each source precondition to it.
	result := []*artifacts_proto.Artifact{}
	for _, source := range artifact.Sources {
		new_artifact := proto.Clone(artifact).(*artifacts_proto.Artifact)
		new_artifact.Sources = []*artifacts_proto.ArtifactSource{source}
		new_artifact.Precondition = source.Precondition
		result = append(result, new_artifact)
	}
	return result
}

// Compile a single artifact, resolve dependencies and tools
func (self *Launcher) GetVQLCollectorArgs(
	ctx context.Context,
	config_obj *config_proto.Config,
	repository services.Repository,
	artifact *artifacts_proto.Artifact,
	spec *flows_proto.ArtifactSpec,
	options services.CompilerOptions) (*actions_proto.VQLCollectorArgs, error) {

	vql_collector_args := &actions_proto.VQLCollectorArgs{}
	err := self.CompileSingleArtifact(config_obj, options, artifact, vql_collector_args)
	if err != nil {
		return nil, err
	}

	err = self.EnsureToolsDeclared(ctx, config_obj, artifact)
	if err != nil {
		return nil, err
	}

	// Add any artifact dependencies.
	err = PopulateArtifactsVQLCollectorArgs(
		ctx, config_obj, repository, vql_collector_args)
	if err != nil {
		return nil, err
	}

	err = self.AddArtifactCollectorArgs(vql_collector_args, spec)
	if err != nil {
		return nil, err
	}

	for _, tool := range artifact.Tools {
		err = AddToolDependency(ctx, config_obj, tool.Name, vql_collector_args)
		if err != nil {
			return nil, err
		}
	}

	if options.ObfuscateNames {
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
	collector_request *flows_proto.ArtifactCollectorArgs,
	completion func()) (string, error) {

	args := collector_request.CompiledCollectorArgs
	if args == nil {
		// Compile and cache the compilation for next time
		// just in case this request is reused.

		// NOTE: We assume that compiling the artifact is a
		// pure function so caching is appropriate.
		compiled, err := self.CompileCollectorArgs(
			ctx, config_obj, acl_manager, repository,
			services.CompilerOptions{
				ObfuscateNames: true,
			}, collector_request)
		if err != nil {
			return "", err
		}
		args = append(args, compiled...)
	}

	return ScheduleArtifactCollectionFromCollectorArgs(
		config_obj, collector_request, args, completion)
}

func ScheduleArtifactCollectionFromCollectorArgs(
	config_obj *config_proto.Config,
	collector_request *flows_proto.ArtifactCollectorArgs,
	vql_collector_args []*actions_proto.VQLCollectorArgs,
	completion func()) (string, error) {

	client_id := collector_request.ClientId
	if client_id == "" {
		return "", errors.New("Client id not provided.")
	}

	db, err := datastore.GetDB(config_obj)
	if err != nil {
		return "", err
	}

	client_manager, err := services.GetClientInfoManager()
	if err != nil {
		return "", err
	}

	session_id := NewFlowId(client_id)

	// Compile all the requests into specific tasks to be sent to the
	// client.
	tasks := []*crypto_proto.VeloMessage{}
	for id, arg := range vql_collector_args {
		// If sending to the server record who actually launched this.
		if client_id == "server" {
			arg.Principal = collector_request.Creator
		}

		// The task we will schedule for the client.
		task := &crypto_proto.VeloMessage{
			QueryId:         uint64(id),
			SessionId:       session_id,
			RequestId:       constants.ProcessVQLResponses,
			VQLClientAction: arg,
		}

		// Send an urgent request to the client.
		if collector_request.Urgent {
			task.Urgent = true
		}

		tasks = append(tasks, task)
	}

	// Save the collection context first.
	flow_path_manager := paths.NewFlowPathManager(client_id, session_id)

	// Generate a new collection context for this flow.
	collection_context := &flows_proto.ArtifactCollectorContext{
		SessionId:           session_id,
		CreateTime:          uint64(time.Now().UnixNano() / 1000),
		State:               flows_proto.ArtifactCollectorContext_RUNNING,
		Request:             collector_request,
		ClientId:            client_id,
		OutstandingRequests: int64(len(tasks)),
	}

	// Store the collection_context first, then queue all the tasks.
	err = db.SetSubjectWithCompletion(config_obj,
		flow_path_manager.Path(),
		collection_context,

		func() {
			// Queue and notify the client about the new tasks
			client_manager.QueueMessagesForClient(
				client_id, tasks, true /* notify */)
		})
	if err != nil {
		return "", err
	}

	// Record the tasks for provenance of what we actually did.
	err = db.SetSubjectWithCompletion(config_obj,
		flow_path_manager.Task(),
		&api_proto.ApiFlowRequestDetails{Items: tasks}, nil)
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

	services.RegisterLauncher(&Launcher{})

	wg.Add(1)
	go func() {
		defer wg.Done()
		defer services.RegisterLauncher(nil)

		<-ctx.Done()

		logger := logging.GetLogger(config_obj, &logging.FrontendComponent)
		logger.Info("Exiting Launcher Service")
	}()

	return nil
}
