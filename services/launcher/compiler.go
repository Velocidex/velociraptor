package launcher

import (
	"context"
	"fmt"
	"path"
	"regexp"
	"sort"
	"strings"

	"github.com/go-errors/errors"
	actions_proto "www.velocidex.com/golang/velociraptor/actions/proto"
	artifacts_proto "www.velocidex.com/golang/velociraptor/artifacts/proto"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	"www.velocidex.com/golang/velociraptor/services"
	vql_subsystem "www.velocidex.com/golang/velociraptor/vql"
	"www.velocidex.com/golang/vfilter"
)

var (
	artifact_in_query_regex = regexp.MustCompile(`Artifact\.([^\s\(]+)\(`)
	escape_regex            = regexp.MustCompile("(^[0-9]|[\"' .-])")
)

func escape_name(name string) string {
	return regexp.MustCompile("[^a-zA-Z0-9]").ReplaceAllString(name, "_")
}

func maybeEscape(name string) string {
	if escape_regex.FindString(name) != "" {
		return "`" + name + "`"
	}
	return name
}

func (self *Launcher) CompileSingleArtifact(
	ctx context.Context, config_obj *config_proto.Config,
	options services.CompilerOptions,
	artifact *artifacts_proto.Artifact,
	repository services.Repository,
	result *actions_proto.VQLCollectorArgs) error {

	// Allow the artifact to dictate the effective user.
	result.EffectivePrincipal = artifact.Impersonate

	for _, parameter := range artifact.Parameters {
		value := parameter.Default
		name := parameter.Name

		env := &actions_proto.VQLEnv{
			Key:   name,
			Value: value,
		}

		// If the parameter has a type, convert it
		// appropriately. Note that parameters are always
		// passed into the client as strings, so they need to
		// be converted into their declared types explicitly
		// in the VQL code.

		// If the variable contains spaces we need to escape
		// the name in backticks.
		escaped_name := maybeEscape(name)

		switch parameter.Type {
		case "", "string", "regex", "yara":
			// Nothing to do with these types.

		case "redacted":
			env.Comment = "redacted"

		case "upload":
			result.Query = append(result.Query, &actions_proto.VQLRequest{
				VQL: fmt.Sprintf(`LET %v <= if(condition=%v, then={
   SELECT Content FROM http_client(url=%v)
})`,
					maybeEscape(name+"_"), escaped_name, escaped_name),
			})
			result.Query = append(result.Query, &actions_proto.VQLRequest{
				VQL: fmt.Sprintf("LET %v <= %v.Content[0]",
					escaped_name, maybeEscape(name+"_")),
			})

		case "upload_file":
			result.Query = append(result.Query, &actions_proto.VQLRequest{
				VQL: fmt.Sprintf(`LET %v <= if(condition=%v, then={
   SELECT Content FROM http_client(url=%v, tempfile_extension='.tmp')
})`,
					maybeEscape(name+"_"), escaped_name, escaped_name),
			})
			result.Query = append(result.Query, &actions_proto.VQLRequest{
				VQL: fmt.Sprintf("LET %v <= %v.Content[0]",
					escaped_name, maybeEscape(name+"_")),
			})

		case "server_metadata":
			client_info_manager, err := services.GetClientInfoManager(config_obj)
			if err == nil {
				md, err := client_info_manager.GetMetadata(ctx, "server")
				if err == nil {
					value, pres := md.GetString(name)
					if pres {
						env.Value = value
					}
				}
			}

		case "int", "int64", "integer":
			result.Query = append(result.Query, &actions_proto.VQLRequest{
				VQL: fmt.Sprintf("LET %v <= int(int=%v)", escaped_name,
					escaped_name),
			})

		case "float":
			result.Query = append(result.Query, &actions_proto.VQLRequest{
				VQL: fmt.Sprintf("LET %v <= parse_float(string=%v)", escaped_name,
					escaped_name),
			})

		case "timestamp":
			result.Query = append(result.Query, &actions_proto.VQLRequest{
				VQL: fmt.Sprintf("LET %v <= timestamp(epoch=%v)", escaped_name,
					escaped_name),
			})
		case "starlark":
			result.Query = append(result.Query, &actions_proto.VQLRequest{
				VQL: fmt.Sprintf(`
LET %v <= if(
    condition=format(format="%%T", args=[%v,]) =~ "string",
    then=starl(code=%v),
    else=%v)
`,
					escaped_name, escaped_name, escaped_name, escaped_name)})
		case "csv", "artifactset":
			// Only parse from CSV if it is a string.
			result.Query = append(result.Query, &actions_proto.VQLRequest{
				VQL: fmt.Sprintf(`
LET %v <= SELECT * FROM if(
    condition=format(format="%%T", args=[%v,]) =~ "string",
    then={SELECT * FROM parse_csv(filename=%v, accessor='data')},
    else=%v)
`,
					escaped_name, escaped_name, escaped_name, escaped_name),
			})

			// Only parse from JSON if it is a string.
		case "json":
			result.Query = append(result.Query, &actions_proto.VQLRequest{
				VQL: fmt.Sprintf(`
LET %v <= if(
    condition=format(format="%%T", args=[%v,]) =~ "string",
    then=parse_json(data=%v),
    else=%v)
`,
					escaped_name, escaped_name, escaped_name, escaped_name),
			})

		case "json_array", "regex_array", "multichoice":
			result.Query = append(result.Query, &actions_proto.VQLRequest{
				VQL: fmt.Sprintf(`
LET %v <= if(
    condition=format(format="%%T", args=[%v,]) = "string",
    then=parse_json_array(data=%v),
    else=%v)
`,
					escaped_name, escaped_name, escaped_name, escaped_name),
			})

		case "xml":
			result.Query = append(result.Query, &actions_proto.VQLRequest{
				VQL: fmt.Sprintf(`
LET %v <= if(
    condition=format(format="%%T", args=[%v,]) =~ "string",
    then=parse_xml(file=%v, accessor="data"),
    else=%v)
`,
					escaped_name, escaped_name, escaped_name, escaped_name),
			})

		case "yaml":
			result.Query = append(result.Query, &actions_proto.VQLRequest{
				VQL: fmt.Sprintf(`
LET %v <= if(
    condition=format(format="%%T", args=[%v,]) =~ "string",
    then=parse_yaml(filename=%v, accessor="data"),
    else=%v)
`,
					escaped_name, escaped_name, escaped_name, escaped_name),
			})

		case "bool":
			result.Query = append(result.Query, &actions_proto.VQLRequest{
				VQL: fmt.Sprintf("LET %v <= get(field='%v') = TRUE OR get(field='%v') =~ '^(Y|TRUE|YES|OK)$' ",
					escaped_name, name, name),
			})

		}

		result.Env = append(result.Env, env)
	}

	// Apply artifact default resource controls.
	if artifact.Resources != nil {
		result.Timeout = artifact.Resources.Timeout
		result.OpsPerSecond = artifact.Resources.OpsPerSecond
		result.CpuLimit = artifact.Resources.CpuLimit
		result.IopsLimit = artifact.Resources.IopsLimit
	}

	err := resolveImports(ctx, config_obj, artifact, repository, result)
	if err != nil {
		return err
	}

	return mergeSources(config_obj, options, artifact, result)
}

func resolveImports(
	ctx context.Context, config_obj *config_proto.Config,
	artifact *artifacts_proto.Artifact,
	repository services.Repository,
	result *actions_proto.VQLCollectorArgs) error {
	// Resolve imports if needed. First check if the artifact
	// itself declares exports for itself (by default each
	// artifact imports its own exports).
	if artifact.Export != "" {
		scope := vql_subsystem.MakeScope()

		// Support multiple queries in the export section.
		queries, err := vfilter.MultiParse(artifact.Export)
		if err != nil {
			return fmt.Errorf("While parsing export in %s: %w",
				artifact.Name, err)
		}

		for _, q := range queries {
			result.Query = append(result.Query,
				&actions_proto.VQLRequest{
					VQL: vfilter.FormatToString(scope, q),
				})
		}
	}

	if artifact.Imports == nil {
		return nil
	}

	// These are a list of names to be imported.
	for _, imported := range artifact.Imports {
		scope := vql_subsystem.MakeScope()

		dependent_artifact, pres := repository.Get(ctx, config_obj, imported)
		if !pres {
			return fmt.Errorf("Artifact %v imports %v which is not known.",
				artifact.Name, imported)
		}
		if dependent_artifact.Export != "" {
			queries, err := vfilter.MultiParse(dependent_artifact.Export)
			if err != nil {
				return fmt.Errorf("While parsing export in %s: %w",
					artifact.Name, err)
			}

			for _, q := range queries {
				result.Query = append(result.Query,
					&actions_proto.VQLRequest{
						VQL: vfilter.FormatToString(scope, q),
					})
			}
		}
	}

	return nil
}

func mergeSources(
	config_obj *config_proto.Config,
	options services.CompilerOptions,
	artifact *artifacts_proto.Artifact,
	result *actions_proto.VQLCollectorArgs) error {

	scope := vql_subsystem.MakeScope()

	precondition := artifact.Precondition
	precondition_var := ""
	if options.DisablePrecondition {
		precondition = ""
	}

	result.Precondition = precondition

	for idx, source := range artifact.Sources {
		// If the source has specialized name and description
		// we use it otherwise take the name and description
		// from the artifact itself. This allows us to create
		// an artifact pack which contains multiple related
		// artifacts in the sources list.

		// NOTE: The client does not receive the actual name
		// or description because we compress the
		// VQLCollectorArgs object before we send it to them
		// (i.e. substitute the strings with place holders).
		// It is therefore safe to include confidential
		// information in the description or name properties
		// of an artifact (Although obviously the client can
		// see the actual VQL query that it is running).
		name := artifact.Name
		if source.Name != "" {
			name = path.Join(name, source.Name)
		}

		prefix := fmt.Sprintf("%s_%d", escape_name(name), idx)
		source_result := ""

		// TODO: This is still here for old clients - new
		// clients do not need it as they will honor the
		// precondition field directly.
		if precondition != "" {
			precondition_var = "precondition_" + prefix
			result.Query = append(result.Query,
				&actions_proto.VQLRequest{
					VQL: "LET " + precondition_var + " = " +
						precondition,
				})
		}

		// An empty query is not an error.
		if strings.TrimSpace(source.Query) == "" {
			continue
		}

		// The artifact format requires all queries to be LET
		// queries except for the last one.
		queries, err := vfilter.MultiParse(source.Query)
		if err != nil {
			return fmt.Errorf("While parsing source query %v: %w",
				source.Name, err)
		}

		for idx2, vql := range queries {
			query_name := fmt.Sprintf("%s_%d", prefix, idx2)
			if idx2 < len(queries)-1 {
				result.Query = append(result.Query,
					&actions_proto.VQLRequest{
						VQL: vfilter.FormatToString(scope, vql),
					})
			} else {
				result.Query = append(result.Query,
					&actions_proto.VQLRequest{
						VQL: "LET " + query_name +
							" = " + vfilter.FormatToString(scope, vql),
					})
			}
			source_result = query_name
		}

		// TODO: Backwards compatibility for older clients.
		if precondition != "" {
			result.Query = append(result.Query, &actions_proto.VQLRequest{
				Name: name,
				VQL: fmt.Sprintf(
					"SELECT * FROM if(then=%s, condition=%s, else={SELECT * FROM scope() WHERE log(message='Query skipped due to precondition') AND FALSE})",
					source_result, precondition_var),
			})
		} else {
			result.Query = append(result.Query, &actions_proto.VQLRequest{
				Name: name,
				VQL:  "SELECT * FROM " + source_result,
			})
		}
	}

	return nil
}

// Parse the query and determine if it requires any artifacts. If any
// artifacts are found, then recursivly determine their dependencies
// etc.
func GetQueryDependencies(
	ctx context.Context, config_obj *config_proto.Config,
	repository services.Repository,
	query string,
	depth int,
	dependency map[string]int) error {

	// For now this is really dumb - just search for something
	// that looks like an artifact.
	for _, hit := range artifact_in_query_regex.
		FindAllStringSubmatch(query, -1) {
		artifact_name := hit[1]
		dep, pres := repository.Get(ctx, config_obj, artifact_name)
		if !pres {
			return errors.New(
				fmt.Sprintf("Unknown artifact reference %s",
					artifact_name))
		}

		_, pres = dependency[hit[1]]
		if pres {
			continue
		}

		dependency[artifact_name] = depth

		if dep.Export != "" {
			err := GetQueryDependencies(ctx, config_obj, repository,
				dep.Export, 0, dependency)
			if err != nil {
				return err
			}
		}

		// Add any artifact that this one imports as a dependency.
		for _, imp := range dep.Imports {
			_, pres = dependency[imp]
			if pres {
				continue
			}

			dependency[imp] = depth
			imported_artifact, pres := repository.Get(ctx, config_obj, imp)
			if !pres {
				return fmt.Errorf(
					"Imported Artifact %v not found (needed by %v)",
					imp, artifact_name)
			}

			// If the exported section depends on other artifacts,
			// then add them too.
			err := GetQueryDependencies(ctx, config_obj, repository,
				imported_artifact.Export, 0, dependency)
			if err != nil {
				return err
			}
		}

		// Now search the referred to artifact's query for its
		// own dependencies.
		err := GetQueryDependencies(ctx,
			config_obj, repository, dep.Precondition, depth+1, dependency)
		if err != nil {
			return err
		}

		for _, source := range dep.Sources {
			err := GetQueryDependencies(ctx, config_obj, repository,
				source.Precondition, depth+1, dependency)
			if err != nil {
				return err
			}

			err = GetQueryDependencies(ctx, config_obj, repository,
				source.Query, depth+1, dependency)
			if err != nil {
				return err
			}
		}
	}

	return nil
}

// Attach additional artifacts to the request if needed to satisfy
// dependencies.
func PopulateArtifactsVQLCollectorArgs(
	ctx context.Context,
	config_obj *config_proto.Config,
	repository services.Repository,
	request *actions_proto.VQLCollectorArgs) error {
	dependencies := make(map[string]int)
	for _, query := range request.Query {
		err := GetQueryDependencies(ctx, config_obj, repository,
			query.VQL, 0, dependencies)
		if err != nil {
			return err
		}
	}

	// Sort dependencies for output stability.
	dep_names := make([]string, 0, len(dependencies))
	for k := range dependencies {
		dep_names = append(dep_names, k)
	}
	sort.Strings(dep_names)

	for _, k := range dep_names {
		artifact, pres := repository.Get(ctx, config_obj, k)
		if pres {
			// Filter the artifact to contain only
			// essential data.
			sources := []*artifacts_proto.ArtifactSource{}
			for _, source := range artifact.Sources {
				new_source := &artifacts_proto.ArtifactSource{
					Name:         source.Name,
					Precondition: stripComments(source.Precondition),
					Queries:      source.Queries,
				}
				sources = append(sources, new_source)
			}

			// Deliberately make a copy of the artifact -
			// we do not want to give away metadata to the
			// client. Only pass the bare necessary
			// details of the definition.
			filtered_parameters := make(
				[]*artifacts_proto.ArtifactParameter, 0,
				len(artifact.Parameters))
			for _, param := range artifact.Parameters {
				filtered_parameters = append(filtered_parameters,
					&artifacts_proto.ArtifactParameter{
						Name:    param.Name,
						Type:    param.Type,
						Default: param.Default,
					})
			}

			// Sub artifacts run in an isolated scope so
			// the main artifact's env is not visibile to
			// them. In the case of tools, we want the
			// tool parameters to be visible to all sub
			// artifacts as well. We therefore copy these
			// into the artifact definitions as
			// parameters. Note that dependent artifacts
			// never declare their own tools themselves
			// since we dont want them to fetch the tool
			// independently.
			tmp := &actions_proto.VQLCollectorArgs{}
			for _, tool := range artifact.Tools {
				err := AddToolDependency(ctx, config_obj, tool.Name, tool.Version, tmp)
				if err != nil {
					return err
				}
			}

			for _, env := range tmp.Env {
				parameter := &artifacts_proto.ArtifactParameter{
					Name:    env.Key,
					Default: env.Value,
				}
				filtered_parameters = append(filtered_parameters, parameter)
			}

			request.Artifacts = append(request.Artifacts,
				&artifacts_proto.Artifact{
					Name:         artifact.Name,
					Type:         artifact.Type,
					Precondition: stripComments(artifact.Precondition),
					Export:       stripComments(artifact.Export),
					Imports:      artifact.Imports,
					Parameters:   filtered_parameters,
					Sources:      sources,

					// Do not pass tool
					// definitions to the
					// client. Otherwise they will
					// be added to it's local
					// inventory and confuse the
					// next request.
					Tools: nil,
				})
		}
	}

	return nil
}

// Given a set of artifacts, returns the wider set of these artifacts
// and any other artifacts that may potentially be used by these. We
// sending requests to the client we never use the client's embedded
// artifacts. Instead the server must provide all dependencies in the
// request. This function is used to fill in a copy of the dependent
// artifacts in the client request.
func (self *Launcher) GetDependentArtifacts(
	ctx context.Context, config_obj *config_proto.Config,
	repository services.Repository, names []string) ([]string, error) {

	dependency := make(map[string]int)

	for _, name := range names {
		if name == "" {
			continue
		}

		_, pres := dependency[name]
		if pres {
			continue
		}

		_, pres = repository.Get(ctx, config_obj, name)
		if !pres {
			return nil, fmt.Errorf(
				"GetDependentArtifacts: Artifact %v not found", name)
		}

		err := GetQueryDependencies(ctx, config_obj, repository,
			fmt.Sprintf("SELECT * FROM Artifact.%s()", name), 0, dependency)
		if err != nil {
			return nil, err
		}
	}

	result := make([]string, 0, len(dependency))
	for k := range dependency {
		result = append(result, k)
	}

	return result, nil
}

// Reformat the VQL if possible to remove any comments.
func stripComments(query string) string {
	if query == "" {
		return ""
	}

	scope := vql_subsystem.MakeScope()
	vqls, err := vfilter.MultiParse(query)
	if err != nil {
		return query
	}

	result := []string{}
	for _, vql := range vqls {
		result = append(result, vfilter.FormatToString(scope, vql))
	}
	return strings.Join(result, "\n")
}
