package collector

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/Velocidex/ordereddict"
	"www.velocidex.com/golang/velociraptor/accessors/file"
	"www.velocidex.com/golang/velociraptor/acls"
	actions_proto "www.velocidex.com/golang/velociraptor/actions/proto"
	"www.velocidex.com/golang/velociraptor/artifacts"
	artifacts_proto "www.velocidex.com/golang/velociraptor/artifacts/proto"
	"www.velocidex.com/golang/velociraptor/config"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	"www.velocidex.com/golang/velociraptor/file_store/csv"
	flows_proto "www.velocidex.com/golang/velociraptor/flows/proto"
	"www.velocidex.com/golang/velociraptor/json"
	"www.velocidex.com/golang/velociraptor/reporting"
	"www.velocidex.com/golang/velociraptor/services"
	"www.velocidex.com/golang/velociraptor/utils"
	vql_subsystem "www.velocidex.com/golang/velociraptor/vql"
	"www.velocidex.com/golang/velociraptor/vql/functions"
	"www.velocidex.com/golang/vfilter"
	"www.velocidex.com/golang/vfilter/arg_parser"
)

var (
	Clock utils.Clock = utils.RealClock{}
)

type CollectPluginArgs struct {
	Artifacts           []string            `vfilter:"optional,field=artifacts,doc=A list of artifacts to collect."`
	Output              string              `vfilter:"optional,field=output,doc=A path to write the output file on."`
	Report              string              `vfilter:"optional,field=report,doc=A path to write the report on (deprecated and ignored)."`
	Args                vfilter.Any         `vfilter:"optional,field=args,doc=Optional parameters."`
	Password            string              `vfilter:"optional,field=password,doc=An optional password to encrypt the collection zip."`
	Format              string              `vfilter:"optional,field=format,doc=Output format (csv, jsonl, csv_only)."`
	ArtifactDefinitions vfilter.Any         `vfilter:"optional,field=artifact_definitions,doc=Optional additional custom artifacts."`
	Template            string              `vfilter:"optional,field=template,doc=(Deprecated Ignored)."`
	Level               int64               `vfilter:"optional,field=level,doc=Compression level between 0 (no compression) and 9."`
	OpsPerSecond        int64               `vfilter:"optional,field=ops_per_sec,doc=Rate limiting for collections (deprecated)."`
	CpuLimit            float64             `vfilter:"optional,field=cpu_limit,doc=Set query cpu_limit value"`
	IopsLimit           float64             `vfilter:"optional,field=iops_limit,doc=Set query iops_limit value"`
	ProgressTimeout     float64             `vfilter:"optional,field=progress_timeout,doc=If no progress is detected in this many seconds, we terminate the query and output debugging information"`
	Timeout             float64             `vfilter:"optional,field=timeout,doc=Total amount of time in seconds, this collection will take. Collection is cancelled when timeout is exceeded."`
	Metadata            vfilter.StoredQuery `vfilter:"optional,field=metadata,doc=Metadata to store in the zip archive. Outputs to metadata.json in top level of zip file."`
	Concurrency         int64               `vfilter:"optional,field=concurrency,doc=Number of concurrent collections."`
	Remapping           string              `vfilter:"optional,field=remapping,doc=A Valid remapping configuration in YAML or JSON format."`
}

type CollectPlugin struct{}

func (self CollectPlugin) Call(
	ctx context.Context,
	scope vfilter.Scope,
	args *ordereddict.Dict) <-chan vfilter.Row {
	output_chan := make(chan vfilter.Row)

	go func() {
		defer close(output_chan)
		defer vql_subsystem.RegisterMonitor(ctx, "collect", args)()

		// This plugin allows one to create files (for the output
		// zip), It is very privileged.
		err := vql_subsystem.CheckAccess(scope, acls.FILESYSTEM_WRITE)
		if err != nil {
			scope.Log("collect: %s", err)
			return
		}

		arg := &CollectPluginArgs{}
		err = arg_parser.ExtractArgsWithContext(ctx, scope, args, arg)
		if err != nil {
			scope.Log("collect: %v", err)
			return
		}

		config_obj, ok := vql_subsystem.GetServerConfig(scope)
		if !ok {
			config_obj = config.GetDefaultConfig()
		}

		collection_manager := newCollectionManager(ctx, config_obj,
			output_chan, int(arg.Concurrency), scope)

		collection_manager.remapping = arg.Remapping

		// Make sure the Close() is called under any circumstances.
		err = vql_subsystem.GetRootScope(scope).AddDestructor(func() {
			collection_manager.Close()
		})
		if err != nil {
			scope.Log("collect: %v", err)
		}

		defer func() {
			err := collection_manager.Close()
			if err != nil {
				scope.Log("collect: While closing container: %v", err)
			}
		}()

		request, err := self.configureCollection(
			ctx, scope, collection_manager, arg)
		if err != nil {
			scope.Log("collect: %v", err)
			return
		}

		err = collection_manager.Collect(request)
		if err != nil {
			scope.Log("collect: %v", err)
			return
		}

	}()

	return output_chan
}

// Configures the collection manager with the provided args.
func (self CollectPlugin) configureCollection(
	ctx context.Context,
	scope vfilter.Scope,
	manager *collectionManager,
	arg *CollectPluginArgs) (*flows_proto.ArtifactCollectorArgs, error) {

	format, err := reporting.GetContainerFormat(arg.Format)
	if err != nil {
		return nil, err
	}

	// Set the output format
	err = manager.SetFormat(format)
	if err != nil {
		return nil, err
	}

	// Add any additional artifacts from the provided definitions into
	// a temporary repository.
	err = manager.GetRepository(arg.ArtifactDefinitions)
	if err != nil {
		return nil, err
	}

	// Set any timeout if needed.
	if arg.Timeout > 0 {
		// arg.Timeout is in sec and we need ns
		manager.SetTimeout(arg.Timeout * 1e9)
	}

	// Apply a throttler if needed.
	manager.AddThrottler(float64(arg.OpsPerSecond), arg.CpuLimit,
		arg.IopsLimit, arg.ProgressTimeout)

	// If required create an output container
	if arg.Output != "" {

		// Make sure we are allowed to write there.
		err = file.CheckPath(arg.Output)
		if err != nil {
			return nil, err
		}

		// Set any metadata for the container.
		if !utils.IsNil(arg.Metadata) {
			manager.SetMetadata(arg.Metadata)
		}

		// Build the container to receive the output from the
		// queries. The container may be password protected.
		err = manager.MakeContainer(arg.Output, arg.Password, arg.Level)
		if err != nil {
			return nil, err
		}
	}

	// Compile the request into vql requests protobuf ready for
	// acquisition.
	return getArtifactCollectorArgs(ctx,
		manager.config_obj, manager.repository, manager.scope, arg)
}

func (self CollectPlugin) Info(scope vfilter.Scope, type_map *vfilter.TypeMap) *vfilter.PluginInfo {
	return &vfilter.PluginInfo{
		Name:     "collect",
		Doc:      "Collect artifacts into a local file.",
		ArgType:  type_map.AddType(scope, &CollectPluginArgs{}),
		Metadata: vql_subsystem.VQLMetadata().Permissions(acls.FILESYSTEM_WRITE).Build(),
		Version:  2,
	}
}

// Parse the plugin arg into an artifact collector arg that can be
// compiled into VQL requests
func getArtifactCollectorArgs(
	ctx context.Context, config_obj *config_proto.Config,
	repository services.Repository,
	scope vfilter.Scope,
	arg *CollectPluginArgs) (res *flows_proto.ArtifactCollectorArgs, err error) {

	request := &flows_proto.ArtifactCollectorArgs{}

	request.Specs, request.Artifacts, err = AddSpecProtobuf(
		ctx, config_obj, repository, scope, arg.Args, arg.Artifacts)
	if err != nil {
		return nil, err
	}

	if len(request.Artifacts) == 0 {
		return nil, fmt.Errorf("No artifacts to collect!")
	}

	return request, nil
}

// Builds a spec protobuf from the arg.Args that was passed. Note that
// artifact parameters are always strings, encoded according to the
// parameter type.
func AddSpecProtobuf(
	ctx context.Context, config_obj *config_proto.Config,
	repository services.Repository,
	scope vfilter.Scope,
	spec vfilter.Any,
	artifacts []string) (res []*flows_proto.ArtifactSpec,
	artifact_names []string, err error) {

	spec_dict, err := utils.ConvertToArtifactSpecs(spec)
	if err != nil {
		return nil, nil, err
	}

	seen := make(map[string]bool)

	for _, item := range spec_dict.Items() {
		artifact_definitions, pres := repository.Get(ctx, config_obj, item.Key)
		if !pres {
			// Artifact not known
			return nil, nil, fmt.Errorf(`Parameter refers to an unknown artifact (%v). The parameter should be of the form {"Custom.Artifact.Name":{"arg":"value"}}`, item.Key)
		}

		// Check that we are allowed to collect this artifact
		err := CheckArtifactCollection(scope, artifact_definitions)
		if err != nil {
			return nil, nil, err
		}

		spec_proto := &flows_proto.ArtifactSpec{
			Artifact:   item.Key,
			Parameters: &flows_proto.ArtifactParameters{},
		}

		spec_parameters, pres := item.Value.(*ordereddict.Dict)
		if !pres {
			return nil, nil, fmt.Errorf(`Parameters of artifact (%v) have incorrect types. The parameter should be of the form {"Custom.Artifact.Name":{"arg":"value"}}`, item.Key)
		}

		// The parameters dict provided in the spec is a
		// key/value dict with key being the parameter name
		// and value being either a string or a value which
		// will be converted to a string according to the
		// parameter type.
		for _, parameter_definition := range artifact_definitions.Parameters {
			// Check if the spec specifies a value for this parameter
			value_any, pres := scope.Associative(
				spec_parameters, parameter_definition.Name)
			if !pres {
				continue
			}

			value_str, is_str := value_any.(string)

			// It is not a string, convert to
			// string according to the parameter
			// type
			switch parameter_definition.Type {
			case "", "string", "regex":
				if !is_str {
					scope.Log("Parameter %v should be a string",
						parameter_definition.Name)
					continue
				}
			case "int", "int64":
				value_str = fmt.Sprintf("%v", value_any)

			case "bool":
				if is_str {
					switch value_str {
					case "Y", "TRUE", "true", "True":
						value_str = "Y"
					default:
						value_str = ""
					}
				} else {
					if scope.Bool(value_any) {
						value_str = "Y"
					} else {
						value_str = ""
					}
				}

			case "choices":
				if !is_str {
					scope.Log("Parameter %v should be a string",
						parameter_definition.Name)
					continue
				}

				valid_choice := false
				for _, choice := range parameter_definition.Choices {
					if choice == value_str {
						valid_choice = true
					}
				}

				if !valid_choice {
					scope.Log("Invalid choice for parameter %v: %v",
						parameter_definition.Name, value_str)
				}

			case "timestamp":
				if !is_str && !utils.IsNil(value_any) {
					value_time, err := functions.TimeFromAny(ctx, scope, value_any)
					if err != nil {
						scope.Log("Invalid timestamp for %v",
							parameter_definition.Name)
						continue
					}
					value_str = value_time.UTC().String()
				}

			case "csv":
				if !is_str {
					value_str, err = csv.EncodeToCSV(
						config_obj, scope, value_any,
						json.DefaultEncOpts())
					if err != nil {
						scope.Log("Invalid CSV for %v",
							parameter_definition.Name)
						continue
					}
				}

			case "json", "json_array":
				if !is_str {
					value, err := json.MarshalIndent(value_any)
					if err != nil {
						return nil, nil, err
					}
					value_str = string(value)
				}
			}

			spec_proto.Parameters.Env = append(spec_proto.Parameters.Env,
				&actions_proto.VQLEnv{
					Key:   parameter_definition.Name,
					Value: value_str,
				})
		}

		res = append(res, spec_proto)
		artifact_names = append(artifact_names, spec_proto.Artifact)
		seen[spec_proto.Artifact] = true

	}

	for _, artifact := range artifacts {
		_, pres := seen[artifact]
		if !pres {
			res = append(res, &flows_proto.ArtifactSpec{
				Artifact: artifact,
			})
			artifact_names = append(artifact_names, artifact)
		}
	}

	return res, artifact_names, nil
}

// Check if the artifact can be added or modified.
func CheckArtifactModification(
	scope vfilter.Scope,
	artifact *artifacts_proto.Artifact) error {

	var ok bool
	var err error

	acl_manager, err := artifacts.GetACLManager(scope)
	if err != nil {
		return err
	}

	switch strings.ToUpper(artifact.Type) {
	case "CLIENT", "CLIENT_EVENT":
		ok, err = acl_manager.CheckAccess(acls.ARTIFACT_WRITER)
		if !ok {
			return errors.New("Permission denied: ARTIFACT_WRITER")
		}

	case "SERVER", "SERVER_EVENT", "NOTEBOOK", "INTERNAL":
		ok, err = acl_manager.CheckAccess(acls.SERVER_ARTIFACT_WRITER)
		if !ok {
			return errors.New("Permission denied: SERVER_ARTIFACT_WRITER")
		}

	default:
		return errors.New("Unknown artifact type for permission check")
	}

	return err
}

// Check if the artifact can be added or modified.
func CheckArtifactCollection(
	scope vfilter.Scope,
	artifact *artifacts_proto.Artifact) error {

	var ok bool
	var err error

	acl_manager, err := artifacts.GetACLManager(scope)
	if err != nil {
		return err
	}

	switch strings.ToUpper(artifact.Type) {
	case "CLIENT", "CLIENT_EVENT":
		ok, err = acl_manager.CheckAccess(acls.COLLECT_CLIENT)
		if !ok {
			return errors.New("Permission denied: COLLECT_CLIENT")
		}

	case "SERVER", "SERVER_EVENT", "NOTEBOOK", "INTERNAL":
		ok, err = acl_manager.CheckAccess(acls.COLLECT_SERVER)
		if !ok {
			return errors.New("Permission denied: COLLECT_SERVER")
		}

	default:
		return errors.New("Unknown artifact type for permission check")
	}

	return err
}

func init() {
	vql_subsystem.RegisterPlugin(&CollectPlugin{})
}
