package server

import (
	"context"
	"strings"

	"github.com/Velocidex/ordereddict"
	"google.golang.org/protobuf/proto"
	"www.velocidex.com/golang/velociraptor/acls"
	artifacts_proto "www.velocidex.com/golang/velociraptor/artifacts/proto"
	"www.velocidex.com/golang/velociraptor/json"
	"www.velocidex.com/golang/velociraptor/services"
	"www.velocidex.com/golang/velociraptor/vql"
	vql_subsystem "www.velocidex.com/golang/velociraptor/vql"
	vql_utils "www.velocidex.com/golang/velociraptor/vql/utils"
	"www.velocidex.com/golang/vfilter"
	"www.velocidex.com/golang/vfilter/arg_parser"
)

const (
	REPOSITORY_CACHE_TAG = "__REPOSITORY_"
)

type ArtifactSetFunctionArgs struct {
	Definition string   `vfilter:"optional,field=definition,doc=Artifact definition in YAML"`
	Prefix     string   `vfilter:"optional,field=prefix,doc=Optional name prefix (deprecated ignored)"`
	Tags       []string `vfilter:"optional,field=tags,doc=Optional tags to attach to the artifact."`
	Repository string   `vfilter:"optional,field=repository,doc=Add the artifact to this repository, if not set, we add the artifact to the global repository."`
}

type ArtifactSetFunction struct{}

func (self *ArtifactSetFunction) Call(ctx context.Context,
	scope vfilter.Scope,
	args *ordereddict.Dict) vfilter.Any {

	arg := &ArtifactSetFunctionArgs{}
	err := arg_parser.ExtractArgsWithContext(ctx, scope, args, arg)
	if err != nil {
		scope.Log("artifact_set: %v", err)
		return vfilter.Null{}
	}

	// Allow artifacts to be set on the client outside the frontend.
	config_obj, ok := vql_subsystem.GetServerConfig(scope)
	if !ok {
		scope.Log("artifact_set: Command can only run on the server")
		return vfilter.Null{}
	}

	manager, _ := services.GetRepositoryManager(config_obj)
	if manager == nil {
		scope.Log("artifact_set: Command can only run on the server")
		return vfilter.Null{}
	}

	tmp_repository := manager.NewRepository()
	definition, err := tmp_repository.LoadYaml(arg.Definition,
		services.ArtifactOptions{
			ValidateArtifact:  true,
			ArtifactIsBuiltIn: false,
		})
	if err != nil {
		definition := arg.Definition
		if len(arg.Definition) > 100 {
			definition = arg.Definition[:99] + " ..."
		}
		scope.Log("artifact_set: %v: %v", err, definition)
		return vfilter.Null{}
	}

	// Determine the permission required based on the type of the artifact.
	var permission acls.ACL_PERMISSION

	def_type := strings.ToLower(definition.Type)

	switch def_type {
	case "client", "client_event", "":
		permission = acls.ARTIFACT_WRITER
	case "server", "server_event", "notebook":
		permission = acls.SERVER_ARTIFACT_WRITER
	default:
		scope.Log("artifact_set: artifact type %v invalid", definition.Type)
		return vfilter.Null{}
	}

	err = vql_subsystem.CheckAccess(scope, permission)
	if err != nil {
		scope.Log("artifact_set: %s", err)
		return vfilter.Null{}
	}

	principal := vql_subsystem.GetPrincipal(scope)

	global_repository, err := manager.GetGlobalRepository(config_obj)
	if err != nil {
		scope.Log("artifact_set: %s", err)
		return vfilter.Null{}
	}

	if arg.Repository != "" {
		var local_repository services.Repository
		cached_any := vql_subsystem.CacheGet(scope, REPOSITORY_CACHE_TAG+arg.Repository)

		if cached_repository, ok := cached_any.(services.Repository); ok {
			local_repository = cached_repository
		} else {
			scope.Log("artifact_set: creating new repository '%s'", arg.Repository)
			local_repository = manager.NewRepository()
			local_repository.SetParent(global_repository, config_obj)
		}

		// Determine if this is a built-in artifact
		tmp_repository := local_repository.Copy()
		built_in := false

		artifact, err := tmp_repository.LoadYaml(arg.Definition,
			services.ArtifactOptions{
				ValidateArtifact:  true,
				ArtifactIsBuiltIn: true,
			})
		if err == nil {
			if global_artifact, pres := global_repository.Get(ctx, config_obj, artifact.Name); pres {
				built_in = global_artifact.BuiltIn
			}
		}

		definition, err := local_repository.LoadYaml(arg.Definition,
			services.ArtifactOptions{
				ValidateArtifact:  true,
				ArtifactIsBuiltIn: built_in,
			})
		if err != nil {
			scope.Log("artifact_set: %s", err)
			return vfilter.Null{}
		}

		scope.Log("artifact_set: added %s to repository '%s'", definition.Name, arg.Repository)
		vql_subsystem.CacheSet(scope, REPOSITORY_CACHE_TAG+arg.Repository, local_repository)

		return json.ConvertProtoToOrderedDict(definition)
	}

	definition, err = manager.SetArtifactFile(ctx,
		config_obj, principal, arg.Definition, arg.Prefix)
	if err != nil {
		scope.Log("artifact_set: %s", err)
		return vfilter.Null{}
	}

	if len(arg.Tags) > 0 {
		metadata := definition.Metadata
		if metadata == nil {
			metadata = &artifacts_proto.ArtifactMetadata{}
		}

		metadata.Tags = arg.Tags

		err = manager.SetArtifactMetadata(ctx, config_obj,
			principal, definition.Name, metadata)
		if err != nil {
			scope.Log("artifact_set: %s", err)
			return vfilter.Null{}
		}
	}

	return json.ConvertProtoToOrderedDict(definition)
}

func (self ArtifactSetFunction) Info(
	scope vfilter.Scope, type_map *vfilter.TypeMap) *vfilter.FunctionInfo {
	return &vfilter.FunctionInfo{
		Name:    "artifact_set",
		Doc:     "Sets an artifact into the global repository.",
		ArgType: type_map.AddType(scope, &ArtifactSetFunctionArgs{}),
		Metadata: vql.VQLMetadata().Permissions(
			acls.ARTIFACT_WRITER, acls.SERVER_ARTIFACT_WRITER).Build(),
	}
}

type ArtifactDeleteFunctionArgs struct {
	Name string `vfilter:"optional,field=name,doc=The Artifact to delete"`
}

type ArtifactDeleteFunction struct{}

func (self *ArtifactDeleteFunction) Call(ctx context.Context,
	scope vfilter.Scope,
	args *ordereddict.Dict) vfilter.Any {

	arg := &ArtifactDeleteFunctionArgs{}
	err := arg_parser.ExtractArgsWithContext(ctx, scope, args, arg)
	if err != nil {
		scope.Log("artifact_delete: %v", err)
		return vfilter.Null{}
	}

	config_obj, ok := vql_subsystem.GetServerConfig(scope)
	if !ok {
		scope.Log("artifact_delete: Command can only run on the server")
		return vfilter.Null{}
	}

	global_repository, err := vql_utils.GetRepository(scope)
	if err != nil {
		scope.Log("artifact_delete: %v", err)
		return vfilter.Null{}
	}

	definition, pres := global_repository.Get(ctx, config_obj, arg.Name)
	if !pres {
		scope.Log("artifact_delete: Artifact '%v' not found", arg.Name)
		return vfilter.Null{}
	}

	if definition.BuiltIn {
		scope.Log("artifact_delete: Can only delete custom artifacts.")
		return vfilter.Null{}
	}

	var permission acls.ACL_PERMISSION
	def_type := strings.ToLower(definition.Type)

	switch def_type {
	case "client", "client_event", "notebook", "":
		permission = acls.ARTIFACT_WRITER
	case "server", "server_event":
		permission = acls.SERVER_ARTIFACT_WRITER
	}

	err = vql_subsystem.CheckAccess(scope, permission)
	if err != nil {
		scope.Log("artifact_set: %s", err)
		return vfilter.Null{}
	}

	manager, err := services.GetRepositoryManager(config_obj)
	if err != nil {
		scope.Log("artifact_set: %s", err)
		return vfilter.Null{}
	}

	principal := vql_subsystem.GetPrincipal(scope)
	err = manager.DeleteArtifactFile(ctx, config_obj, principal, arg.Name)
	if err != nil {
		scope.Log("artifact_delete: %s", err)
		return vfilter.Null{}
	}

	return arg.Name
}

func (self ArtifactDeleteFunction) Info(
	scope vfilter.Scope, type_map *vfilter.TypeMap) *vfilter.FunctionInfo {
	return &vfilter.FunctionInfo{
		Name:    "artifact_delete",
		Doc:     "Deletes an artifact from the global repository.",
		ArgType: type_map.AddType(scope, &ArtifactDeleteFunctionArgs{}),
		Metadata: vql.VQLMetadata().Permissions(
			acls.ARTIFACT_WRITER, acls.SERVER_ARTIFACT_WRITER).Build(),
	}
}

type ArtifactsPluginArgs struct {
	Names               []string `vfilter:"optional,field=names,doc=Artifact definitions to dump"`
	IncludeDependencies bool     `vfilter:"optional,field=deps,doc=If true includes all dependencies as well."`
	Sanitize            bool     `vfilter:"optional,field=sanitize,doc=If true we remove extra metadata."`
}

type ArtifactsPlugin struct{}

func (self ArtifactsPlugin) Call(
	ctx context.Context,
	scope vfilter.Scope,
	args *ordereddict.Dict) <-chan vfilter.Row {
	output_chan := make(chan vfilter.Row)
	go func() {
		defer close(output_chan)
		defer vql_subsystem.RegisterMonitor(ctx, "artifact_definitions", args)()

		err := vql_subsystem.CheckAccess(scope, acls.READ_RESULTS)
		if err != nil {
			scope.Log("artifact_definitions: %v", err)
			return
		}

		arg := &ArtifactsPluginArgs{}
		err = arg_parser.ExtractArgsWithContext(ctx, scope, args, arg)
		if err != nil {
			scope.Log("artifact_definitions: %v", err)
			return
		}

		config_obj, ok := vql_subsystem.GetServerConfig(scope)
		if !ok {
			scope.Log("artifact_definitions: Command can only run on the server")
			return
		}

		repository, err := vql_utils.GetRepository(scope)
		if err != nil {
			scope.Log("artifact_definitions: %v", err)
			return
		}

		// No args means just dump all artifacts
		if len(arg.Names) == 0 {
			names, err := repository.List(ctx, config_obj)
			if err != nil {
				scope.Log("artifact_definitions: %v", err)
				return
			}
			for _, name := range names {
				artifact, pres := repository.Get(ctx, config_obj, name)
				if !pres {
					continue
				}

				// Clean up the artifact by removing internal fields.
				artifact = proto.Clone(artifact).(*artifacts_proto.Artifact)
				for _, source := range artifact.Sources {
					if source.Query != "" && len(source.Queries) > 0 {
						source.Queries = nil
					}
				}

				if pres {
					select {
					case <-ctx.Done():
						return
					case output_chan <- json.ConvertProtoToOrderedDict(artifact):
					}
				}
			}
			return
		}

		seen := make(map[string]*artifacts_proto.Artifact)
		for _, name := range arg.Names {
			artifact, pres := repository.Get(ctx, config_obj, name)
			if !pres {
				scope.Log("artifact_definitions: artifact %v not known", name)
				continue
			}
			seen[artifact.Name] = artifact
		}

		launcher, err := services.GetLauncher(config_obj)
		if err != nil {
			scope.Log("artifact_definitions: Command can only run on the server %v", err)
			return
		}

		if arg.IncludeDependencies {
			deps, err := launcher.GetDependentArtifacts(ctx,
				config_obj, repository, arg.Names)
			if err != nil {
				scope.Log("artifact_definitions: %v", err)
				return
			}

			for _, name := range deps {
				if name == "" {
					continue
				}
				artifact, pres := repository.Get(ctx, config_obj, name)
				if pres {
					seen[artifact.Name] = artifact
				}
			}
		}

		for _, artifact := range seen {
			select {
			case <-ctx.Done():
				return
			case output_chan <- json.ConvertProtoToOrderedDict(artifact):
			}
		}
	}()

	return output_chan
}

func (self ArtifactsPlugin) Info(scope vfilter.Scope, type_map *vfilter.TypeMap) *vfilter.PluginInfo {
	return &vfilter.PluginInfo{
		Name:     "artifact_definitions",
		Doc:      "Dump artifact definitions.",
		ArgType:  type_map.AddType(scope, &ArtifactsPluginArgs{}),
		Metadata: vql.VQLMetadata().Permissions(acls.READ_RESULTS).Build(),
	}
}

type ArtifactSetMetadataFunctionArgs struct {
	Name   string   `vfilter:"required,field=name,doc=The Artifact to update"`
	Hidden bool     `vfilter:"optional,field=hidden,doc=Set to true make the artifact hidden in the GUI, false to make it visible again."`
	Basic  bool     `vfilter:"optional,field=basic,doc=Set to true make the artifact a 'basic' artifact. This allows users with the COLLECT_BASIC permission able to collect it."`
	Tags   []string `vfilter:"optional,field=tags,doc=Optional tags to attach to the artifact."`
}

type ArtifactSetMetadataFunction struct{}

func (self *ArtifactSetMetadataFunction) Call(ctx context.Context,
	scope vfilter.Scope,
	args *ordereddict.Dict) vfilter.Any {

	arg := &ArtifactSetMetadataFunctionArgs{}
	err := arg_parser.ExtractArgsWithContext(ctx, scope, args, arg)
	if err != nil {
		scope.Log("artifact_set_metadata: %v", err)
		return vfilter.Null{}
	}

	err = services.RequireFrontend()
	if err != nil {
		scope.Log("artifact_set_metadata: %v", err)
		return vfilter.Null{}
	}

	config_obj, ok := vql_subsystem.GetServerConfig(scope)
	if !ok {
		scope.Log("artifact_set_metadata: Command can only run on the server")
		return vfilter.Null{}
	}

	global_repository, err := vql_utils.GetRepository(scope)
	if err != nil {
		scope.Log("artifact_set_metadata: %v", err)
		return vfilter.Null{}
	}

	definition, pres := global_repository.Get(ctx, config_obj, arg.Name)
	if !pres {
		scope.Log("artifact_set_metadata: Artifact '%v' not found", arg.Name)
		return vfilter.Null{}
	}

	metadata := definition.Metadata
	if metadata == nil {
		metadata = &artifacts_proto.ArtifactMetadata{}
	}

	var permission acls.ACL_PERMISSION
	def_type := strings.ToLower(definition.Type)

	switch def_type {
	case "client", "client_event", "notebook", "":
		permission = acls.ARTIFACT_WRITER
	case "server", "server_event", "internal":
		permission = acls.SERVER_ARTIFACT_WRITER
	}

	err = vql_subsystem.CheckAccess(scope, permission)
	if err != nil {
		scope.Log("artifact_set_metadata: %s", err)
		return vfilter.Null{}
	}

	// Need to explicitly check if the arg is passed at all or just
	// false.
	_, pres = args.Get("hidden")
	if pres {
		metadata.Hidden = arg.Hidden
	}

	_, pres = args.Get("basic")
	if pres {
		metadata.Basic = arg.Basic
	}

	// Override the tags if specified.
	_, pres = args.Get("tags")
	if pres {
		metadata.Tags = arg.Tags
	}

	principal := vql_subsystem.GetPrincipal(scope)
	manager, err := services.GetRepositoryManager(config_obj)
	if err != nil {
		scope.Log("artifact_set_metadata: %s", err)
		return vfilter.Null{}
	}

	err = manager.SetArtifactMetadata(ctx, config_obj, principal, arg.Name, metadata)
	if err != nil {
		scope.Log("artifact_set_metadata: %s", err)
		return vfilter.Null{}
	}

	return metadata
}

func (self ArtifactSetMetadataFunction) Info(
	scope vfilter.Scope, type_map *vfilter.TypeMap) *vfilter.FunctionInfo {
	return &vfilter.FunctionInfo{
		Name:    "artifact_set_metadata",
		Doc:     "Sets metadata about the artifact.",
		ArgType: type_map.AddType(scope, &ArtifactSetMetadataFunctionArgs{}),
		Metadata: vql.VQLMetadata().Permissions(
			acls.ARTIFACT_WRITER, acls.SERVER_ARTIFACT_WRITER).Build(),
	}
}

type ArtifactImportFunctionArgs struct {
	Artifact string `vfilter:"required,field=artifact,doc=The Artifact to import"`
}

type ArtifactImportFunction struct{}

func (self *ArtifactImportFunction) Call(ctx context.Context,
	scope vfilter.Scope,
	args *ordereddict.Dict) vfilter.Any {

	arg := &ArtifactImportFunctionArgs{}
	err := arg_parser.ExtractArgsWithContext(ctx, scope, args, arg)
	if err != nil {
		scope.Log("import: %v", err)
		return vfilter.Null{}
	}

	config_obj, ok := vql_subsystem.GetServerConfig(scope)
	if !ok {
		scope.Log("import: Command can only run on the server")
		return vfilter.Null{}
	}

	global_repository, err := vql_utils.GetRepository(scope)
	if err != nil {
		scope.Log("import: %v", err)
		return vfilter.Null{}
	}

	definition, pres := global_repository.Get(ctx, config_obj, arg.Artifact)
	if !pres {
		scope.Log("import: Artifact '%v' not found", arg.Artifact)
		return vfilter.Null{}
	}

	// Compile the export section
	if definition.Export != "" {
		vqls, err := vfilter.MultiParse(definition.Export)
		if err != nil {
			scope.Log("import: Artifact '%v': %v", arg.Artifact, err)
			return vfilter.Null{}
		}

		// Do not do anything with the rows since exports are not
		// supposed to actually return rows (they should be only LET
		// statements).
		for _, vql := range vqls {
			for _ = range vql.Eval(ctx, scope) {
			}
		}
	}

	return definition.Export
}

func (self ArtifactImportFunction) Info(
	scope vfilter.Scope, type_map *vfilter.TypeMap) *vfilter.FunctionInfo {
	return &vfilter.FunctionInfo{
		Name:     "import",
		Doc:      "Imports an artifact into the current scope. This only works in notebooks!",
		ArgType:  type_map.AddType(scope, &ArtifactImportFunctionArgs{}),
		Metadata: vql.VQLMetadata().Build(),
	}
}

func init() {
	vql_subsystem.RegisterFunction(&ArtifactImportFunction{})
	vql_subsystem.RegisterFunction(&ArtifactSetMetadataFunction{})
	vql_subsystem.RegisterPlugin(&ArtifactsPlugin{})
	vql_subsystem.RegisterFunction(&ArtifactSetFunction{})
	vql_subsystem.RegisterFunction(&ArtifactDeleteFunction{})
}
