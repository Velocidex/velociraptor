package tools

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/Velocidex/ordereddict"
	"www.velocidex.com/golang/velociraptor/acls"
	"www.velocidex.com/golang/velociraptor/actions"
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
	"www.velocidex.com/golang/velociraptor/vql/acl_managers"
	"www.velocidex.com/golang/velociraptor/vql/functions"
	"www.velocidex.com/golang/vfilter"
	"www.velocidex.com/golang/vfilter/arg_parser"
)

type CollectPluginArgs struct {
	Artifacts           []string    `vfilter:"required,field=artifacts,doc=A list of artifacts to collect."`
	Output              string      `vfilter:"optional,field=output,doc=A path to write the output file on."`
	Report              string      `vfilter:"optional,field=report,doc=A path to write the report on."`
	Args                vfilter.Any `vfilter:"optional,field=args,doc=Optional parameters."`
	Password            string      `vfilter:"optional,field=password,doc=An optional password to encrypt the collection zip."`
	Format              string      `vfilter:"optional,field=format,doc=Output format (csv, jsonl)."`
	ArtifactDefinitions vfilter.Any `vfilter:"optional,field=artifact_definitions,doc=Optional additional custom artifacts."`
	Template            string      `vfilter:"optional,field=template,doc=The name of a template artifact (i.e. one which has report of type HTML)."`
	Level               int64       `vfilter:"optional,field=level,doc=Compression level between 0 (no compression) and 9."`
	OpsPerSecond        int64       `vfilter:"optional,field=ops_per_sec,doc=Rate limiting for collections (deprecated)."`
	CpuLimit            float64     `vfilter:"optional,field=cpu_limit,doc=Set query cpu_limit value"`
	IopsLimit           float64     `vfilter:"optional,field=iops_limit,doc=Set query iops_limit value"`
	ProgressTimeout     float64     `vfilter:"optional,field=progress_timeout,doc=If no progress is detected in this many seconds, we terminate the query and output debugging information"`
	Timeout             float64     `vfilter:"optional,field=timeout,doc=Total amount of time in seconds, this collection will take. Collection is cancelled when timeout is exceeded."`
}

type CollectPlugin struct{}

func (self CollectPlugin) Call(
	ctx context.Context,
	scope vfilter.Scope,
	args *ordereddict.Dict) <-chan vfilter.Row {
	output_chan := make(chan vfilter.Row)

	go func() {
		defer close(output_chan)

		var container *reporting.Container
		var closer func()

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

		switch arg.Format {
		case "jsonl", "csv", "json":
		case "":
			arg.Format = "jsonl"
		default:
			scope.Log("collect: format %v not supported", arg.Format)
			return
		}

		if arg.Template == "" {
			arg.Template = "Reporting.Default"
		}

		config_obj, ok := vql_subsystem.GetServerConfig(scope)
		if !ok {
			config_obj = config.GetDefaultConfig()
		}

		// Get a new artifact repository with extra definitions added.
		repository, err := getRepository(scope, config_obj, arg.ArtifactDefinitions)
		if err != nil {
			scope.Log("collect: %v", err)
			return
		}

		// Compile the request into vql requests protobuf
		request, err := getArtifactCollectorArgs(config_obj, repository, scope, arg)
		if err != nil {
			scope.Log("collect: %v", err)
			return
		}

		// Run the query with a potential subctx with timeout and
		// cancellation different than our own.
		subctx, cancel := context.WithCancel(ctx)
		defer cancel()

		// Apply a timeout if requested.
		if arg.Timeout > 0 {
			start := time.Now()
			timed_ctx, timed_cancel := context.WithTimeout(
				ctx, time.Duration(arg.Timeout*1e9)*time.Nanosecond)

			go func() {
				select {
				case <-ctx.Done():
					timed_cancel()

				case <-timed_ctx.Done():
					scope.Log("collect: <red>Timeout Error:</> Collection timed out after %v",
						time.Now().Sub(start))
					// Cancel the main context.
					cancel()
					timed_cancel()
				}
			}()
		}

		// Create the output container
		if arg.Output != "" {
			container, closer, err = makeContainer(ctx,
				config_obj, scope, repository, arg)
			if err != nil {
				scope.Log("collect: %v", err)
				return
			}

			// If the query is interrupted we may not get to run the
			// closer function. Add to scope destructors to ensure it
			// gets called at query wind-down and that we wait for it
			// to actually close.
			vql_subsystem.GetRootScope(scope).AddDestructor(closer)

			// When we exit, close the container and flush the
			// name to the output channel.
			defer func() {
				// Close the container.
				closer()

				// Emit the result set for consumption by the
				// rest of the query.
				select {
				case <-subctx.Done():
					return

				case output_chan <- ordereddict.NewDict().
					Set("Container", arg.Output).
					Set("Report", arg.Report):
				}
			}()
		}

		// Create a sub scope to run the new collection in -
		// based on our existing scope but override the
		// uploader with the container.
		builder := services.ScopeBuilderFromScope(scope)
		builder.Uploader = container

		// When run within an ACL context, copy the ACL
		// manager to the subscope - otherwise the user can
		// bypass the ACL manager and get more permissions.
		acl_manager, ok := artifacts.GetACLManager(scope)
		if !ok {
			acl_manager = acl_managers.NullACLManager{}
		}

		launcher, err := services.GetLauncher(config_obj)
		if err != nil {
			scope.Log("collect: %v", err)
			return
		}

		vql_requests, err := launcher.CompileCollectorArgs(
			subctx, config_obj, acl_manager, repository,
			services.CompilerOptions{}, request)
		if err != nil {
			scope.Log("collect: %v", err)
			return
		}

		// Run each collection separately, one after the other.
		for _, vql_request := range vql_requests {

			// Make a new scope for each artifact.
			manager, err := services.GetRepositoryManager(config_obj)
			if err != nil {
				scope.Log("collect: %v", err)
				return
			}

			// Create a new environment for each request.
			env := ordereddict.NewDict()
			for _, env_spec := range vql_request.Env {
				env.Set(env_spec.Key, env_spec.Value)
			}

			subscope := manager.BuildScope(builder)
			subscope.AppendVars(env)
			defer subscope.Close()

			// Install throttler into the scope.
			throttler := actions.NewThrottler(subctx, scope,
				float64(arg.OpsPerSecond),
				float64(arg.CpuLimit),
				float64(arg.IopsLimit))

			if arg.ProgressTimeout > 0 {
				throttler = actions.NewProgressThrottler(
					subctx, scope, cancel, throttler,
					time.Duration(arg.ProgressTimeout*1e9)*time.Nanosecond)
			}

			subscope.SetThrottler(throttler)

			// Run each query and store the results in the container
			for _, query := range vql_request.Query {
				// Useful to know what is going on with the collection.
				if query.Name != "" {
					subscope.Log("Starting collection of %s", query.Name)
				}

				// If there is no container we just
				// return the rows to our caller.
				if container == nil {
					query_log := actions.QueryLog.AddQuery(query.VQL)

					vql, err := vfilter.Parse(query.VQL)
					if err != nil {
						subscope.Log("collect: %v", err)
						return
					}
					for row := range vql.Eval(subctx, subscope) {
						output_chan <- row
					}
					query_log.Close()

					continue
				}

				err = container.StoreArtifact(
					config_obj, subctx, subscope, query, arg.Format)
				if err != nil {
					subscope.Log("collect: %v", err)
					return
				}

				if query.Name != "" {
					subscope.Log("Collected %s", query.Name)
				}
			}
		}
	}()

	return output_chan
}

// Creates a container to write the results on. Results are completed
// when container is closed.
func makeContainer(
	ctx context.Context,
	config_obj *config_proto.Config,
	scope vfilter.Scope,
	repository services.Repository,
	arg *CollectPluginArgs) (
	container *reporting.Container, closer func(), err error) {
	// Should we encrypt it?
	if arg.Password != "" {
		scope.Log("Will password protect container")
	}

	scope.Log("Setting compression level to %v", arg.Level)

	container, err = reporting.NewContainer(
		config_obj, arg.Output, arg.Password, arg.Level)
	if err != nil {
		return nil, nil, err
	}

	scope.Log("Will create container at %s", arg.Output)

	// On exit we create a report.
	closer = func() {
		if container.IsClosed() {
			return
		}
		container.Close()

		if arg.Report != "" {
			scope.Log("Producing collection report at %v", arg.Report)

			// Open the archive back up again. // TODO: Support password.
			archive, err := reporting.NewArchiveReader(arg.Output)
			if err != nil {
				scope.Log("Error opening archive: %v", err)
				return
			}
			defer archive.Close()

			// Produce a report for each of the collected
			// artifacts.
			definitions := []*artifacts_proto.Artifact{}
			for _, name := range arg.Artifacts {
				artifact, pres := repository.Get(config_obj, name)
				if !pres {
					scope.Log("Artifact %v not known.", name)
					return
				}
				definitions = append(definitions, artifact)
			}

			fd, err := os.OpenFile(
				arg.Report, os.O_RDWR|os.O_CREATE|os.O_TRUNC, 0700)
			if err != nil {
				scope.Log("Error creating report: %v", err)
				return
			}
			defer fd.Close()

			err = produceReport(ctx,
				config_obj, archive,
				arg.Template,
				repository, fd,
				definitions,
				scope, arg)
			if err != nil {
				scope.Log("Error creating report: %v", err)
			}
		}
	}

	return container, closer, nil
}

func getRepository(
	scope vfilter.Scope,
	config_obj *config_proto.Config,
	extra_artifacts vfilter.Any) (services.Repository, error) {
	manager, err := services.GetRepositoryManager(config_obj)
	if err != nil {
		return nil, err
	}
	repository, err := manager.GetGlobalRepository(config_obj)
	if err != nil {
		return nil, err
	}

	if extra_artifacts == nil {
		return repository, nil
	}

	// Private copy of the repository.
	repository = repository.Copy()

	loader := func(item *ordereddict.Dict) error {
		serialized, err := json.Marshal(item)
		if err != nil {
			return err
		}

		artifact, err := repository.LoadYaml(
			string(serialized), true /* validate */, false)
		if err != nil {
			return err
		}

		// Check if we are allows to add these artifacts
		err = CheckArtifactModification(scope, artifact)
		if err != nil {
			return err
		}

		return nil
	}

	switch t := extra_artifacts.(type) {
	case []*ordereddict.Dict:
		for _, item := range t {
			err := loader(item)
			if err != nil {
				return nil, err
			}
		}

	case *ordereddict.Dict:
		err := loader(t)
		if err != nil {
			return nil, err
		}

	case []string:
		for _, item := range t {
			artifact, err := repository.LoadYaml(item,
				true /* validate */, false /* built_in */)
			if err != nil {
				return nil, err
			}

			err = CheckArtifactModification(scope, artifact)
			if err != nil {
				return nil, err
			}
		}

	case string:
		artifact, err := repository.LoadYaml(t,
			true /* validate */, false /* built_in */)
		if err != nil {
			return nil, err
		}

		err = CheckArtifactModification(scope, artifact)
		if err != nil {
			return nil, err
		}
	}

	return repository, nil
}

func (self CollectPlugin) Info(scope vfilter.Scope, type_map *vfilter.TypeMap) *vfilter.PluginInfo {
	return &vfilter.PluginInfo{
		Name: "collect",
		Doc:  "Collect artifacts into a local file.",
		ArgType: type_map.AddType(scope,
			&CollectPluginArgs{}),
	}
}

// Parse the plugin arg into an artifact collector arg that can be
// compiled into VQL requests
func getArtifactCollectorArgs(
	config_obj *config_proto.Config,
	repository services.Repository,
	scope vfilter.Scope,
	arg *CollectPluginArgs) (*flows_proto.ArtifactCollectorArgs, error) {

	request := &flows_proto.ArtifactCollectorArgs{
		Artifacts: arg.Artifacts,
	}

	err := AddSpecProtobuf(config_obj, repository, scope, arg.Args, request)
	if err != nil {
		return nil, err
	}

	return request, nil
}

func convertToArtifactSpecs(spec vfilter.Any) (*ordereddict.Dict, error) {
	serialized, err := json.Marshal(spec)
	if err != nil {
		return nil, err
	}
	specs := []*flows_proto.ArtifactSpec{}
	err = json.Unmarshal(serialized, &specs)
	if err != nil {
		spec := &flows_proto.ArtifactSpec{}
		err = json.Unmarshal(serialized, spec)
		if err != nil {
			return nil, err
		}
		specs = append(specs, spec)
	}

	spec_dict := ordereddict.NewDict()
	for _, spec := range specs {
		env := ordereddict.NewDict()
		if spec.Parameters != nil {
			for _, item := range spec.Parameters.Env {
				env.Set(item.Key, item.Value)
			}
		}
		spec_dict.Set(spec.Artifact, env)
	}
	return spec_dict, nil
}

// Builds a spec protobuf from the arg.Args that was passed. Note that
// artifact parameters are always strings, encoded according to the
// parameter type.
func AddSpecProtobuf(
	config_obj *config_proto.Config,
	repository services.Repository,
	scope vfilter.Scope, spec vfilter.Any,
	request *flows_proto.ArtifactCollectorArgs) error {

	// The spec might be received from Flow.request.specs already
	// which would make it in protobuf form.
	_, pres := scope.Associative(spec, "artifact")
	if pres {
		spec_dict, err := convertToArtifactSpecs(spec)
		if err == nil {
			spec = spec_dict
		}
	}

	for _, name := range scope.GetMembers(spec) {
		artifact_definitions, pres := repository.Get(config_obj, name)
		if !pres {
			// Artifact not known
			return fmt.Errorf(`Parameter 'args' refers to an unknown artifact (%v). The 'args' parameter should be of the form {"Custom.Artifact.Name":{"arg":"value"}}`, name)
		}

		// Check that we are allowed to collect this artifact
		err := CheckArtifactCollection(scope, artifact_definitions)
		if err != nil {
			return err
		}

		spec_proto := &flows_proto.ArtifactSpec{
			Artifact:   name,
			Parameters: &flows_proto.ArtifactParameters{},
		}

		spec_parameters, pres := scope.Associative(spec, name)
		if !pres {
			continue
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
					value_time, err := functions.TimeFromAny(scope, value_any)
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
						config_obj, scope, value_any)
					if err != nil {
						scope.Log("Invalid CSV for %v",
							parameter_definition.Name)
						continue
					}
				}

			case "json", "json_array":
				if !is_str {
					value_str = json.StringIndent(value_any)
				}
			}

			spec_proto.Parameters.Env = append(spec_proto.Parameters.Env,
				&actions_proto.VQLEnv{
					Key:   parameter_definition.Name,
					Value: value_str,
				})
		}

		request.Specs = append(request.Specs, spec_proto)
	}

	return nil
}

// Check if the artifact can be added or modified.
func CheckArtifactModification(
	scope vfilter.Scope,
	artifact *artifacts_proto.Artifact) error {

	var ok bool
	var err error

	acl_manager, ok := artifacts.GetACLManager(scope)
	if !ok {
		return nil
	}

	switch strings.ToUpper(artifact.Type) {
	case "CLIENT", "CLIENT_EVENT":
		ok, err = acl_manager.CheckAccess(acls.ARTIFACT_WRITER)
		if !ok {
			return errors.New("Permission denied: ARTIFACT_WRITER")
		}

	case "SERVER", "SERVER_EVENT", "INTERNAL":
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

	acl_manager, ok := artifacts.GetACLManager(scope)
	if !ok {
		return nil
	}

	switch strings.ToUpper(artifact.Type) {
	case "CLIENT", "CLIENT_EVENT":
		ok, err = acl_manager.CheckAccess(acls.COLLECT_CLIENT)
		if !ok {
			return errors.New("Permission denied: COLLECT_CLIENT")
		}

	case "SERVER", "SERVER_EVENT", "INTERNAL":
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
