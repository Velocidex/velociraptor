package launcher

import (
	"context"
	"fmt"
	"path"
	"regexp"

	errors "github.com/pkg/errors"
	actions_proto "www.velocidex.com/golang/velociraptor/actions/proto"
	artifacts_proto "www.velocidex.com/golang/velociraptor/artifacts/proto"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	"www.velocidex.com/golang/velociraptor/logging"
	"www.velocidex.com/golang/velociraptor/services"
	"www.velocidex.com/golang/velociraptor/utils"
	vql_subsystem "www.velocidex.com/golang/velociraptor/vql"
	"www.velocidex.com/golang/vfilter"
)

var (
	artifact_in_query_regex = regexp.MustCompile(`Artifact\.([^\s\(]+)\(`)
	escape_regex            = regexp.MustCompile("(^[0-9]|[\"' ])")
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

func CompileSingleArtifact(config_obj *config_proto.Config,
	artifact *artifacts_proto.Artifact,
	result *actions_proto.VQLCollectorArgs) error {
	for _, parameter := range artifact.Parameters {
		value := parameter.Default
		name := parameter.Name

		result.Env = append(result.Env, &actions_proto.VQLEnv{
			Key:   name,
			Value: value,
		})

		// If the parameter has a type, convert it
		// appropriately. Note that parameters are always
		// passed into the client as strings, so they need to
		// be converted into their declared types explicitly
		// in the VQL code.

		// If the variable contains spaces we need to escape
		// the name in backticks.
		escaped_name := maybeEscape(name)

		switch parameter.Type {
		case "int", "int64":
			result.Query = append(result.Query, &actions_proto.VQLRequest{
				VQL: fmt.Sprintf("LET %v <= int(int=%v)", escaped_name,
					escaped_name),
			})
		case "timestamp":
			result.Query = append(result.Query, &actions_proto.VQLRequest{
				VQL: fmt.Sprintf("LET %v <= timestamp(epoch=%v)", escaped_name,
					escaped_name),
			})
		case "csv":
			// Only parse from CSV if it is a string.
			result.Query = append(result.Query, &actions_proto.VQLRequest{
				VQL: fmt.Sprintf(`
LET %v <= SELECT * FROM if(
    condition=format(format="%%T", args=%v) =~ "string",
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
    condition=format(format="%%T", args=%v) =~ "string",
    then=parse_json(data=%v),
    else=%v)
`,
					escaped_name, escaped_name, escaped_name, escaped_name),
			})

		case "json_array":
			result.Query = append(result.Query, &actions_proto.VQLRequest{
				VQL: fmt.Sprintf(`
LET %v <= if(
    condition=format(format="%%T", args=%v) =~ "string",
    then=parse_json_array(data=%v),
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

	}

	// Merge any tools we need.
	for _, required_tool := range artifact.Tools {
		if !utils.InString(result.Tools, required_tool.Name) {
			result.Tools = append(result.Tools, required_tool.Name)
		}
	}

	return mergeSources(config_obj, artifact, result)
}

func mergeSources(
	config_obj *config_proto.Config, artifact *artifacts_proto.Artifact,
	result *actions_proto.VQLCollectorArgs) error {

	scope := vql_subsystem.MakeScope()

	for idx, source := range artifact.Sources {
		// If a precondition is defined at the artifact level, the
		// source may override it.
		source_precondition := artifact.Precondition
		source_precondition_var := ""
		if source.Precondition != "" {
			source_precondition = source.Precondition
		}

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
		description := artifact.Description

		if source.Name != "" {
			name = path.Join(name, source.Name)
		}

		if source.Description != "" {
			description = source.Description
		}

		prefix := fmt.Sprintf("%s_%d", escape_name(name), idx)
		source_result := ""

		if source_precondition != "" {
			source_precondition_var = "precondition_" + prefix
			result.Query = append(result.Query,
				&actions_proto.VQLRequest{
					VQL: "LET " + source_precondition_var + " = " +
						source_precondition,
				})
		}

		// The artifact format requires all queries to be LET
		// queries except for the last one.
		queries, err := vfilter.MultiParse(source.Query)
		if err != nil {
			return errors.Wrap(err, "While parsing source query")
		}

		for idx2, vql := range queries {
			query_name := fmt.Sprintf("%s_%d", prefix, idx2)
			if idx2 < len(queries)-1 {
				if vql.Let == "" {
					return errors.New(
						"Invalid artifact " + artifact.Name +
							": All Queries in a source " +
							"must be LET queries, except for the " +
							"final one.")
				}
				result.Query = append(result.Query,
					&actions_proto.VQLRequest{
						VQL: vql.ToString(scope),
					})
			} else {
				if vql.Let != "" {
					return errors.New(
						"Invalid artifact " + artifact.Name +
							": All Queries in a source " +
							"must be LET queries, except for the " +
							"final one.")
				}

				result.Query = append(result.Query,
					&actions_proto.VQLRequest{
						VQL: "LET " + query_name +
							" = " + vql.ToString(scope),
					})
			}
			source_result = query_name
		}

		if source_precondition != "" {
			result.Query = append(result.Query, &actions_proto.VQLRequest{
				Name:        name,
				Description: description,
				VQL: fmt.Sprintf(
					"SELECT * FROM if(then=%s, condition=%s, else={SELECT * FROM scope() WHERE log(message='Query skipped due to precondition') AND FALSE})",
					source_result, source_precondition_var),
			})
		} else {
			result.Query = append(result.Query, &actions_proto.VQLRequest{
				Name:        name,
				Description: description,
				VQL:         "SELECT * FROM " + source_result,
			})
		}
	}

	return nil
}

// Parse the query and determine if it requires any artifacts. If any
// artifacts are found, then recursivly determine their dependencies
// etc.
func GetQueryDependencies(
	config_obj *config_proto.Config,
	repository services.Repository,
	query string,
	depth int,
	dependency map[string]int) error {

	// For now this is really dumb - just search for something
	// that looks like an artifact.
	for _, hit := range artifact_in_query_regex.
		FindAllStringSubmatch(query, -1) {
		artifact_name := hit[1]
		dep, pres := repository.Get(config_obj, artifact_name)
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

		// Now search the referred to artifact's query for its
		// own dependencies.
		err := GetQueryDependencies(
			config_obj, repository, dep.Precondition, depth+1, dependency)
		if err != nil {
			return err
		}

		for _, source := range dep.Sources {
			err := GetQueryDependencies(config_obj, repository,
				source.Precondition, depth+1, dependency)
			if err != nil {
				return err
			}

			err = GetQueryDependencies(config_obj, repository,
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
	config_obj *config_proto.Config,
	repository services.Repository,
	request *actions_proto.VQLCollectorArgs) error {
	dependencies := make(map[string]int)
	for _, query := range request.Query {
		err := GetQueryDependencies(config_obj, repository,
			query.VQL, 0, dependencies)
		if err != nil {
			return err
		}
	}

	for k := range dependencies {
		artifact, pres := repository.Get(config_obj, k)
		if pres {
			// Include any dependent tools.
			for _, required_tool := range artifact.Tools {
				if !utils.InString(request.Tools, required_tool.Name) {
					request.Tools = append(request.Tools, required_tool.Name)
				}
			}

			// Filter the artifact to contain only
			// essential data.
			sources := []*artifacts_proto.ArtifactSource{}
			for _, source := range artifact.Sources {
				new_source := &artifacts_proto.ArtifactSource{
					Name:    source.Name,
					Queries: source.Queries,
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

			request.Artifacts = append(request.Artifacts,
				&artifacts_proto.Artifact{
					Name:       artifact.Name,
					Parameters: filtered_parameters,
					Sources:    sources,
					Tools:      artifact.Tools,
				})
		}
	}

	return nil
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

func (self *Launcher) GetDependentArtifacts(
	config_obj *config_proto.Config,
	repository services.Repository,
	names []string) ([]string, error) {

	dependency := make(map[string]int)

	for _, name := range names {
		_, pres := dependency[name]
		if pres {
			continue
		}

		_, pres = repository.Get(config_obj, name)
		if !pres {
			return nil, errors.New("Artifact not found")
		}

		err := GetQueryDependencies(config_obj, repository,
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
