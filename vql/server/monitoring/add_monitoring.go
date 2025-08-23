package monitoring

import (
	"context"

	"github.com/Velocidex/ordereddict"
	"www.velocidex.com/golang/velociraptor/acls"
	actions_proto "www.velocidex.com/golang/velociraptor/actions/proto"
	flows_proto "www.velocidex.com/golang/velociraptor/flows/proto"
	"www.velocidex.com/golang/velociraptor/services"
	"www.velocidex.com/golang/velociraptor/vql"
	vql_subsystem "www.velocidex.com/golang/velociraptor/vql"
	vql_utils "www.velocidex.com/golang/velociraptor/vql/utils"
	"www.velocidex.com/golang/vfilter"
	"www.velocidex.com/golang/vfilter/arg_parser"
)

type AddClientMonitoringFunctionArgs struct {
	Artifact   string           `vfilter:"required,field=artifact,doc=The name of the artifact to add"`
	Parameters vfilter.LazyExpr `vfilter:"optional,field=parameters,doc=A dict of artifact parameters"`
	Label      string           `vfilter:"optional,field=label,doc=Add the artifact to this label group (default all)"`
}

type AddClientMonitoringFunction struct{}

func (self AddClientMonitoringFunction) Call(
	ctx context.Context,
	scope vfilter.Scope,
	args *ordereddict.Dict) vfilter.Any {

	err := vql_subsystem.CheckAccess(scope, acls.COLLECT_CLIENT)
	if err != nil {
		scope.Log("add_client_monitoring: %s", err)
		return vfilter.Null{}
	}

	arg := &AddClientMonitoringFunctionArgs{}
	err = arg_parser.ExtractArgsWithContext(ctx, scope, args, arg)
	if err != nil {
		scope.Log("add_client_monitoring: %v", err)
		return vfilter.Null{}
	}

	err = services.RequireFrontend()
	if err != nil {
		scope.Log("add_client_monitoring: %v", err)
		return vfilter.Null{}
	}

	config_obj, ok := vql_subsystem.GetServerConfig(scope)
	if !ok {
		scope.Log("add_client_monitoring: Command can only run on the server")
		return vfilter.Null{}
	}

	// Now verify the artifact actually exists
	repository, err := vql_utils.GetRepository(scope)
	if err != nil {
		scope.Log("add_client_monitoring: %v", err)
		return vfilter.Null{}
	}

	artifact, pres := repository.Get(ctx, config_obj, arg.Artifact)
	if !pres {
		scope.Log("add_client_monitoring: artifact %v not found", arg.Artifact)
		return vfilter.Null{}
	}

	if artifact.Type != "client_event" {
		scope.Log(
			"add_client_monitoring: artifact %v is not a client event artifact",
			arg.Artifact)
		return vfilter.Null{}
	}

	client_event_manager, err := services.ClientEventManager(config_obj)
	if err != nil {
		scope.Log("add_client_monitoring: %v", err)
		return vfilter.Null{}
	}

	event_config := client_event_manager.GetClientMonitoringState()

	label_config := getArtifactCollectorArgs(event_config, arg.Label)

	// First remove the current artifact if it is there already
	removeArtifact(label_config, arg.Artifact)

	// Now add the artifact
	label_config.Artifacts = append(label_config.Artifacts, arg.Artifact)

	// Build a new spec
	new_specs := &flows_proto.ArtifactSpec{
		Artifact:   arg.Artifact,
		Parameters: &flows_proto.ArtifactParameters{},
	}

	if arg.Parameters != nil {
		params := arg.Parameters.Reduce(ctx)
		params_dict, ok := params.(*ordereddict.Dict)
		if !ok {
			scope.Log("add_client_monitoring: parameters should be a dict")
			return vfilter.Null{}
		}

		for _, i := range params_dict.Items() {
			v_str, ok := i.Value.(string)
			if !ok {
				scope.Log(
					"add_client_monitoring: parameter %v should has a string value",
					i.Key)
				return vfilter.Null{}
			}
			new_specs.Parameters.Env = append(new_specs.Parameters.Env,
				&actions_proto.VQLEnv{Key: i.Key, Value: v_str})
		}
	}

	label_config.Specs = append(label_config.Specs, new_specs)

	// Actually set the table
	principal := vql_subsystem.GetPrincipal(scope)
	err = client_event_manager.SetClientMonitoringState(
		ctx, config_obj, principal, event_config)
	if err != nil {
		scope.Log("add_client_monitoring: %v", err)
		return vfilter.Null{}
	}

	return event_config
}

func (self AddClientMonitoringFunction) Info(scope vfilter.Scope, type_map *vfilter.TypeMap) *vfilter.FunctionInfo {
	return &vfilter.FunctionInfo{
		Name:     "add_client_monitoring",
		Doc:      "Adds a new artifact to the client monitoring table.",
		ArgType:  type_map.AddType(scope, &AddClientMonitoringFunctionArgs{}),
		Metadata: vql.VQLMetadata().Permissions(acls.COLLECT_CLIENT).Build(),
	}
}

func getArtifactCollectorArgs(
	config *flows_proto.ClientEventTable, label string) *flows_proto.ArtifactCollectorArgs {
	if label == "" || label == "all" {
		return config.Artifacts
	}

	for _, label_event := range config.LabelEvents {
		if label_event.Label == label {
			return label_event.Artifacts
		}
	}

	// If we get here there is no label group yet so add it.
	label_group := &flows_proto.LabelEvents{
		Label: label, Artifacts: &flows_proto.ArtifactCollectorArgs{},
	}

	config.LabelEvents = append(config.LabelEvents, label_group)
	return label_group.Artifacts
}

func removeArtifact(config *flows_proto.ArtifactCollectorArgs, artifact string) {
	new_names := make([]string, 0, len(config.Artifacts))
	for _, name := range config.Artifacts {
		if name != artifact {
			new_names = append(new_names, name)
		}
	}
	config.Artifacts = new_names

	// Now remove any specs
	new_specs := make([]*flows_proto.ArtifactSpec, 0, len(config.Specs))
	for _, spec := range config.Specs {
		if spec.Artifact != artifact {
			new_specs = append(new_specs, spec)
		}
	}
	config.Specs = new_specs
}

type AddServerMonitoringFunctionArgs struct {
	Artifact   string           `vfilter:"required,field=artifact,doc=The name of the artifact to add"`
	Parameters vfilter.LazyExpr `vfilter:"optional,field=parameters,doc=A dict of artifact parameters"`
}

type AddServerMonitoringFunction struct{}

func (self AddServerMonitoringFunction) Call(
	ctx context.Context,
	scope vfilter.Scope,
	args *ordereddict.Dict) vfilter.Any {

	err := vql_subsystem.CheckAccess(scope, acls.COLLECT_SERVER)
	if err != nil {
		scope.Log("add_server_monitoring: %s", err)
		return vfilter.Null{}
	}

	arg := &AddServerMonitoringFunctionArgs{}
	err = arg_parser.ExtractArgsWithContext(ctx, scope, args, arg)
	if err != nil {
		scope.Log("add_server_monitoring: %v", err)
		return vfilter.Null{}
	}

	err = services.RequireFrontend()
	if err != nil {
		scope.Log("add_server_monitoring: %v", err)
		return vfilter.Null{}
	}

	config_obj, ok := vql_subsystem.GetServerConfig(scope)
	if !ok {
		scope.Log("add_server_monitoring: Command can only run on the server")
		return vfilter.Null{}
	}

	// Now verify the artifact actually exists
	repository, err := vql_utils.GetRepository(scope)
	if err != nil {
		scope.Log("add_server_monitoring: %v", err)
		return vfilter.Null{}
	}

	artifact, pres := repository.Get(ctx, config_obj, arg.Artifact)
	if !pres {
		scope.Log("add_server_monitoring: artifact %v not found", arg.Artifact)
		return vfilter.Null{}
	}

	if artifact.Type != "server_event" {
		scope.Log(
			"add_server_monitoring: artifact %v is not a server event artifact",
			arg.Artifact)
		return vfilter.Null{}
	}

	server_event_manager, err := services.GetServerEventManager(config_obj)
	if err != nil {
		scope.Log("add_server_monitoring: %v", err)
		return vfilter.Null{}
	}

	event_config := server_event_manager.Get()

	// First remove the current artifact if it is there already
	removeArtifact(event_config, arg.Artifact)

	// Now add the artifact
	event_config.Artifacts = append(event_config.Artifacts, arg.Artifact)

	// Build a new spec
	new_specs := &flows_proto.ArtifactSpec{
		Artifact:   arg.Artifact,
		Parameters: &flows_proto.ArtifactParameters{},
	}

	params_dict := ordereddict.NewDict()
	if arg.Parameters != nil {
		params := arg.Parameters.Reduce(ctx)
		params_dict, ok = params.(*ordereddict.Dict)
		if !ok {
			scope.Log("add_client_monitoring: parameters should be a dict")
			return vfilter.Null{}
		}
	}

	for _, i := range params_dict.Items() {
		v_str, ok := i.Value.(string)
		if !ok {
			scope.Log(
				"add_server_monitoring: parameter %v should has a string value",
				i.Key)
			return vfilter.Null{}
		}
		new_specs.Parameters.Env = append(new_specs.Parameters.Env,
			&actions_proto.VQLEnv{Key: i.Key, Value: v_str})
	}
	event_config.Specs = append(event_config.Specs, new_specs)

	// Actually set the table
	principal := vql_subsystem.GetPrincipal(scope)
	err = server_event_manager.Update(ctx, config_obj, principal, event_config)
	if err != nil {
		scope.Log("add_server_monitoring: %v", err)
		return vfilter.Null{}
	}

	return event_config
}

func (self AddServerMonitoringFunction) Info(scope vfilter.Scope, type_map *vfilter.TypeMap) *vfilter.FunctionInfo {
	return &vfilter.FunctionInfo{
		Name:     "add_server_monitoring",
		Doc:      "Adds a new artifact to the server monitoring table.",
		ArgType:  type_map.AddType(scope, &AddServerMonitoringFunctionArgs{}),
		Metadata: vql.VQLMetadata().Permissions(acls.COLLECT_SERVER).Build(),
	}
}

func init() {
	vql_subsystem.RegisterFunction(&AddClientMonitoringFunction{})
	vql_subsystem.RegisterFunction(&AddServerMonitoringFunction{})
}
