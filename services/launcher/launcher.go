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
	"fmt"
	"sync"

	"github.com/go-errors/errors"
	"google.golang.org/protobuf/proto"
	actions_proto "www.velocidex.com/golang/velociraptor/actions/proto"
	"www.velocidex.com/golang/velociraptor/artifacts"
	artifacts_proto "www.velocidex.com/golang/velociraptor/artifacts/proto"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	"www.velocidex.com/golang/velociraptor/constants"
	crypto_proto "www.velocidex.com/golang/velociraptor/crypto/proto"
	flows_proto "www.velocidex.com/golang/velociraptor/flows/proto"
	"www.velocidex.com/golang/velociraptor/json"
	"www.velocidex.com/golang/velociraptor/logging"
	"www.velocidex.com/golang/velociraptor/services"
	"www.velocidex.com/golang/velociraptor/utils"
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

type Launcher struct {
	Storage_ services.FlowStorer
}

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

		// Batching control
		var local_cpu_limit float32
		var max_batch_wait, max_batch_rows uint64
		var max_batch_row_buffer uint64
		var local_timeout uint64

		if config_obj != nil && config_obj.Defaults != nil {
			max_batch_rows = config_obj.Defaults.MaxRows
			max_batch_row_buffer = config_obj.Defaults.MaxRowBufferSize
			max_batch_wait = config_obj.Defaults.MaxBatchWait
		}

		if collector_request.AllowCustomOverrides {
			artifact, _ = repository.Get(ctx, config_obj, "Custom."+spec.Artifact)
		}

		if artifact == nil {
			artifact, _ = repository.Get(ctx, config_obj, spec.Artifact)
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

		// Make sure the user can collect this artifact.
		err := CheckAccess(
			config_obj, artifact, collector_request, acl_manager)
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

			if artifact.Resources.MaxBatchWait > max_batch_wait {
				max_batch_wait = artifact.Resources.MaxBatchWait
			}

			if artifact.Resources.CpuLimit > 0 &&
				artifact.Resources.CpuLimit > local_cpu_limit {
				local_cpu_limit = artifact.Resources.CpuLimit
			}

			if artifact.Resources.MaxBatchRows > max_batch_rows {
				max_batch_rows = artifact.Resources.MaxBatchRows
			}

			if artifact.Resources.MaxBatchRowsBuffer > max_batch_row_buffer {
				max_batch_row_buffer = artifact.Resources.MaxBatchRowsBuffer
			}

			if artifact.Resources.Timeout > local_timeout {
				local_timeout = artifact.Resources.Timeout
			}
		}

		// If the spec specifies a value it overrides the artifact
		// definition
		if spec.CpuLimit > 0 {
			local_cpu_limit = spec.CpuLimit
		}

		if spec.MaxBatchRows > 0 {
			max_batch_rows = spec.MaxBatchRows
		}

		if spec.MaxBatchRowsBuffer > 0 {
			max_batch_row_buffer = spec.MaxBatchRowsBuffer
		}

		if spec.MaxBatchWait > 0 {
			max_batch_wait = spec.MaxBatchWait
		}

		if spec.Timeout > 0 {
			local_timeout = spec.Timeout
		}

		for _, expanded_artifact := range expandArtifacts(artifact) {
			vql_collector_args, err := self.GetVQLCollectorArgs(
				ctx, config_obj, repository, expanded_artifact,
				spec, options)
			if err != nil {
				return nil, err
			}

			if local_cpu_limit > 0 {
				vql_collector_args.CpuLimit = local_cpu_limit
			}

			if local_timeout > 0 {
				vql_collector_args.Timeout = local_timeout
			}

			vql_collector_args.MaxRow = max_batch_rows
			vql_collector_args.MaxWait = max_batch_wait
			vql_collector_args.MaxRowBufferSize = max_batch_row_buffer

			// If the request specifies resource controls
			// they override the defaults.
			if collector_request.OpsPerSecond > 0 {
				vql_collector_args.OpsPerSecond = collector_request.OpsPerSecond
			}

			if collector_request.CpuLimit > 0 {
				vql_collector_args.CpuLimit = collector_request.CpuLimit
			}

			if collector_request.ProgressTimeout > 0 {
				vql_collector_args.ProgressTimeout = collector_request.ProgressTimeout
			}

			if collector_request.IopsLimit > 0 {
				vql_collector_args.IopsLimit = collector_request.IopsLimit
			}

			if vql_collector_args.Timeout == 0 &&
				collector_request.Timeout > 0 {
				vql_collector_args.Timeout = collector_request.Timeout
			}

			if vql_collector_args.MaxRow == 0 {
				vql_collector_args.MaxRow = 1000
			}

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

	// Enforce a max upload limit if it is not specified by anything
	// else.
	if collector_request.MaxUploadBytes == 0 {
		collector_request.MaxUploadBytes = 1024 * 1024 * 1024 // 1Gb
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

	// Update the total count of requests
	for idx, item := range result {
		item.QueryId = int64(idx + 1)
		item.TotalQueries = int64(len(result))
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
	err := self.CompileSingleArtifact(ctx, config_obj,
		options, artifact, repository, vql_collector_args)
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
		err = AddToolDependency(ctx, config_obj, tool.Name,
			tool.Version, vql_collector_args)
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
		inventory, err := services.GetInventory(config_obj)
		if err != nil {
			return err
		}

		_, err = inventory.GetToolInfo(
			ctx, config_obj, tool.Name, tool.Version)
		if err != nil {
			// Add tool info if it is not known but do not
			// override existing tool. This allows the
			// admin to override tools from the artifact
			// itself.
			logger.Info("Adding tool %v from artifact %v",
				tool.Name, artifact.Name)
			err = inventory.AddTool(ctx,
				config_obj, tool,
				services.ToolOptions{
					Upgrade:            true,
					ArtifactDefinition: true,
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
	tool, version string, vql_collector_args *actions_proto.VQLCollectorArgs) error {
	inventory, err := services.GetInventory(config_obj)
	if err != nil {
		return err
	}

	tool_info, err := inventory.GetToolInfo(ctx, config_obj, tool, version)
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

	} else if len(tool_info.ServeUrls) > 0 {
		vql_collector_args.Env = append(vql_collector_args.Env, &actions_proto.VQLEnv{
			Key:   fmt.Sprintf("Tool_%v_URL", tool_info.Name),
			Value: tool_info.ServeUrls[0],
		})

		serialized_urls := json.MustMarshalString(tool_info.ServeUrls)
		vql_collector_args.Env = append(vql_collector_args.Env, &actions_proto.VQLEnv{
			Key:   fmt.Sprintf("Tool_%v_URLs", tool_info.Name),
			Value: serialized_urls,
		})

	} else if tool_info.Url != "" {
		vql_collector_args.Env = append(vql_collector_args.Env, &actions_proto.VQLEnv{
			Key:   fmt.Sprintf("Tool_%v_URL", tool_info.Name),
			Value: tool_info.Url,
		})

		serialized_urls := json.MustMarshalString([]string{tool_info.Url})
		vql_collector_args.Env = append(vql_collector_args.Env, &actions_proto.VQLEnv{
			Key:   fmt.Sprintf("Tool_%v_URLs", tool_info.Name),
			Value: serialized_urls,
		})
	}
	return nil
}

// Scheduling artifact collections only happens on the master node at
// the moment.
func (self *Launcher) ScheduleArtifactCollection(
	ctx context.Context,
	config_obj *config_proto.Config,
	acl_manager vql_subsystem.ACLManager,
	repository services.Repository,
	collector_request *flows_proto.ArtifactCollectorArgs,
	completion func()) (string, error) {

	if !services.IsMaster(config_obj) {
		return "", errors.New(
			"ScheduleArtifactCollection can only be called on the master node")
	}

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

	return self.WriteArtifactCollectionRecord(
		ctx, config_obj, collector_request, args,
		func(task *crypto_proto.VeloMessage) {
			client_manager, err := services.GetClientInfoManager(config_obj)
			if err != nil {
				return
			}

			// Queue and notify the client about the new tasks
			_ = client_manager.QueueMessageForClient(
				ctx, collector_request.ClientId, task,
				services.NOTIFY_CLIENT, completion)
		})
}

func (self *Launcher) WriteArtifactCollectionRecord(
	ctx context.Context,
	config_obj *config_proto.Config,
	collector_request *flows_proto.ArtifactCollectorArgs,
	vql_collector_args []*actions_proto.VQLCollectorArgs,
	completion func(task *crypto_proto.VeloMessage)) (string, error) {

	client_manager, err := services.GetClientInfoManager(config_obj)
	if err != nil {
		return "", err
	}

	client_id := collector_request.ClientId
	err = client_manager.ValidateClientId(client_id)
	if err != nil {
		return "", err
	}

	// If the client id is not known, refuse to schedule messages to
	// it.
	_, err = client_manager.Get(ctx, client_id)
	if err != nil {
		return "", err
	}

	session_id := collector_request.FlowId
	if session_id == "" {
		session_id = utils.NewFlowId(client_id)
	}

	// How long to batch log messages for on the client.
	batch_delay := uint64(2000)
	if collector_request.LogBatchTime > 0 {
		batch_delay = collector_request.LogBatchTime
	} else if config_obj.Frontend != nil &&
		config_obj.Frontend.Resources != nil &&
		config_obj.Frontend.Resources.DefaultLogBatchTime > 0 {
		batch_delay = config_obj.Frontend.Resources.DefaultLogBatchTime
	}

	// Compile all the requests into specific tasks to be sent to the
	// client.
	task := &crypto_proto.VeloMessage{
		SessionId: session_id,
		RequestId: constants.ProcessVQLResponses,
		FlowRequest: &crypto_proto.FlowRequest{
			LogBatchTime:   batch_delay,
			MaxRows:        collector_request.MaxRows,
			MaxLogs:        collector_request.MaxLogs,
			MaxUploadBytes: collector_request.MaxUploadBytes,
		},
	}

	if config_obj.Datastore.Compression == "zlib" {
		task.FlowRequest.Compression = crypto_proto.FlowRequest_ZLIB
	}

	if config_obj.Frontend.CollectionErrorRegex != "" {
		task.FlowRequest.LogErrorRegex = config_obj.Frontend.CollectionErrorRegex
	}

	if collector_request.TraceFreqSec > 0 {
		task.FlowRequest.Trace, err = self.calculateTraceQuery(ctx, config_obj,
			collector_request.TraceFreqSec)
		if err != nil {
			return "", err
		}
	}

	for _, arg := range vql_collector_args {
		// If sending to the server, record who actually launched this.
		if client_id == "server" {
			arg.Principal = collector_request.Creator
		}

		task.FlowRequest.VQLClientActions = append(
			task.FlowRequest.VQLClientActions, arg)
	}

	// Send an urgent request to the client.
	if collector_request.Urgent {
		task.Urgent = true
	}

	// Generate a new collection context for this flow.
	collection_context := &flows_proto.ArtifactCollectorContext{
		SessionId:           session_id,
		CreateTime:          uint64(utils.GetTime().Now().UnixNano() / 1000),
		State:               flows_proto.ArtifactCollectorContext_RUNNING,
		Request:             collector_request,
		ClientId:            client_id,
		TotalRequests:       int64(len(vql_collector_args)),
		OutstandingRequests: int64(len(vql_collector_args)),
	}

	// Record the tasks for provenance of what we actually did.
	err = self.Storage().WriteTask(
		ctx, config_obj, client_id, redactTask(task))
	if err != nil {
		return "", err
	}

	// Run server artifacts inline.
	if client_id == "server" {
		server_artifacts_service, err := services.GetServerArtifactRunner(
			config_obj)
		if err != nil {
			return "", err
		}

		// Write the collection object so the GUI can start tracking
		// it.
		redacted := redactCollectContext(collection_context)
		err = self.Storage().WriteFlow(
			ctx, config_obj, redacted, utils.BackgroundWriter)
		if err != nil {
			return "", err
		}

		// Write the flow on the index.
		err = self.Storage().WriteFlowIndex(ctx, config_obj, redacted)
		if err != nil {
			return "", err
		}

		err = server_artifacts_service.LaunchServerArtifact(
			config_obj, session_id, task.FlowRequest, collection_context)
		return collection_context.SessionId, err
	}

	// Store the collection_context first, then queue all the tasks.
	err = self.Storage().WriteFlow(ctx, config_obj,
		redactCollectContext(collection_context),

		func() {
			completion(task)
		})
	if err != nil {
		return "", err
	}

	// Write the flow on the index.
	err = self.Storage().WriteFlowIndex(ctx, config_obj, collection_context)
	return collection_context.SessionId, err
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
			param.Comment = item.Comment
			return result
		}
	}
	return append(result, param)
}

func NewLauncherService(
	ctx context.Context,
	wg *sync.WaitGroup,
	config_obj *config_proto.Config) (services.Launcher, error) {

	// The laucher service is also created on the client to ensure it
	// can compile artifacts etc. But it does not make sense to
	// actually store any of the flows on the client. We therefore
	// install a dummy storer which just returns errors for any
	// attempts to store flows.
	if config_obj.Datastore == nil {
		return &Launcher{Storage_: &DummyStorer{}}, nil
	}

	res := &Launcher{
		Storage_: NewFlowStorageManager(ctx, config_obj, wg),
	}
	return res, nil
}
