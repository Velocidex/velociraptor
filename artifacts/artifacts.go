/*
   Velociraptor - Hunting Evil
   Copyright (C) 2019 Velocidex Innovations.

   This program is free software: you can redistribute it and/or modify
   it under the terms of the GNU Affero General Public License as published
   by the Free Software Foundation, either version 3 of the License, or
   (at your option) any later version.

   This program is distributed in the hope that it will be useful,
   but WITHOUT ANY WARRANTY; without even the implied warranty of
   MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
   GNU Affero General Public License for more details.

   You should have received a copy of the GNU Affero General Public License
   along with this program.  If not, see <https://www.gnu.org/licenses/>.
*/
package artifacts

import (
	"fmt"
	"io/ioutil"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"sync"

	"github.com/Velocidex/yaml/v2"
	"github.com/golang/protobuf/proto"
	errors "github.com/pkg/errors"
	"www.velocidex.com/golang/velociraptor/acls"
	actions_proto "www.velocidex.com/golang/velociraptor/actions/proto"
	artifacts_proto "www.velocidex.com/golang/velociraptor/artifacts/proto"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	"www.velocidex.com/golang/velociraptor/paths"
	utils "www.velocidex.com/golang/velociraptor/utils"
	vql_subsystem "www.velocidex.com/golang/velociraptor/vql"
	"www.velocidex.com/golang/vfilter"
)

var (
	artifact_in_query_regex = regexp.MustCompile(`Artifact\.([^\s\(]+)\(`)
	global_repository       *Repository
	mu                      sync.Mutex
)

// Holds multiple artifact definitions.
type Repository struct {
	sync.Mutex
	Data        map[string]*artifacts_proto.Artifact
	loaded_dirs []string

	artifact_plugin vfilter.PluginGeneratorInterface
}

func (self *Repository) Copy() *Repository {
	self.Lock()
	defer self.Unlock()

	result := &Repository{Data: make(map[string]*artifacts_proto.Artifact)}
	for k, v := range self.Data {
		result.Data[k] = v
	}
	return result
}

func (self *Repository) LoadDirectory(dirname string) (*int, error) {
	self.Lock()

	count := 0
	if utils.InString(self.loaded_dirs, dirname) {
		return &count, nil
	}
	dirname = filepath.Clean(dirname)
	self.loaded_dirs = append(self.loaded_dirs, dirname)

	self.Unlock()

	result, err := &count, filepath.Walk(dirname,
		func(file_path string, info os.FileInfo, err error) error {
			if err != nil {
				return errors.WithStack(err)
			}

			if !info.IsDir() && (strings.HasSuffix(info.Name(), ".yaml") ||
				strings.HasSuffix(info.Name(), ".yml")) {
				data, err := ioutil.ReadFile(file_path)
				if err != nil {
					return errors.WithStack(err)
				}
				_, err = self.LoadYaml(string(data), false)
				if err != nil {
					return err
				}

				count += 1
			}
			return nil
		})

	return result, err
}

var query_regexp = regexp.MustCompile(`(?im)(^ +- +)(SELECT|LET|//)`)

// Fix common YAML errors.
func sanitize_artifact_yaml(data string) string {
	// YAML has two types of block level scalars. The default one
	// (which is more intuitive to use) does not preserve white
	// space. This leads to terrible rendering in the GUI and
	// elsewhere because the query appears all on the one
	// line. The user should use the literal scalar (i.e. '- |'
	// form) but this is a difficult yaml rule to remember and
	// makes the artifact look terrible.

	// Therefore we just transform one form into the other in
	// order to trick the yaml decoder to do the right thing.

	result := query_regexp.ReplaceAllStringFunc(data, func(m string) string {
		parts := query_regexp.FindStringSubmatch(m)
		return parts[1] + "|\n" + strings.Repeat(" ", len(parts[1])) + parts[2]
	})
	return result

}

func (self *Repository) LoadYaml(data string, validate bool) (
	*artifacts_proto.Artifact, error) {
	artifact := &artifacts_proto.Artifact{}
	err := yaml.UnmarshalStrict([]byte(sanitize_artifact_yaml(data)), artifact)
	if err != nil {
		return nil, errors.WithStack(err)
	}

	artifact.Raw = data
	return self.LoadProto(artifact, validate)
}

func (self *Repository) LoadProto(artifact *artifacts_proto.Artifact, validate bool) (
	*artifacts_proto.Artifact, error) {
	self.Lock()
	defer self.Unlock()

	// Validate the artifact.
	for _, report := range artifact.Reports {
		report.Type = strings.ToLower(report.Type)
		switch report.Type {
		case "monitoring_daily", "server_event", "client",
			"internal", "hunt":

		case "html": // HTML reports form a main HTML page for report exports.
		default:
			return nil, errors.New(fmt.Sprintf("Invalid report type %s",
				report.Type))
		}
	}

	// Normalize the type.
	artifact.Type = strings.ToLower(artifact.Type)
	switch artifact.Type {
	case "":
		// By default use the client type.
		artifact.Type = "client"

	case "client", "client_event", "server", "server_event", "internal":
		// These types are acceptable.

	default:
		return nil, errors.New("Artifact type invalid.")
	}

	// Normalize the artifact by converting the deprecated Queries
	// field to the Query field.
	for _, source := range artifact.Sources {
		if source.Query == "" {
			for _, query := range source.Queries {
				source.Query += "\n" + query
			}
		}

		// Remove the Queries field - from now it will contain
		// optimized queries.
		source.Queries = nil
	}

	// Validate the artifact contains syntactically correct
	// VQL. We do not need to validate embedded artifacts since we
	// assume they are ok if they passed CI.
	if validate {
		for _, perm := range artifact.RequiredPermissions {
			if acls.GetPermission(perm) == acls.NO_PERMISSIONS {
				return nil, errors.New("Invalid artifact permission")
			}
		}

		for _, source := range artifact.Sources {
			if source.Precondition != "" {
				_, err := vfilter.Parse(source.Precondition)
				if err != nil {
					return nil, err
				}
			}

			if len(source.Query) == 0 {
				return nil, errors.New(fmt.Sprintf(
					"Source %s in artifact %s contains no queries!",
					source.Name, artifact.Name))
			}

			// Check we can parse it properly.
			_, err := vfilter.MultiParse(source.Query)
			if err != nil {
				return nil, err
			}
		}
	}

	if artifact.Name == "" {
		return nil, errors.New("No artifact name")
	}

	self.Data[artifact.Name] = artifact

	// Clear the cache to force a rebuild.
	self.artifact_plugin = nil

	return artifact, nil
}

func (self *Repository) Get(name string) (*artifacts_proto.Artifact, bool) {
	self.Lock()
	defer self.Unlock()

	result, pres := self.get(name)
	if !pres {
		return nil, false
	}

	// Delay processing until we need it. This means loading
	// artifacts is faster.
	compileArtifact(result)

	// Return a copy to keep the repository pristine.
	return proto.Clone(result).(*artifacts_proto.Artifact), true
}

func (self *Repository) get(name string) (*artifacts_proto.Artifact, bool) {
	artifact_name, source_name := paths.SplitFullSourceName(name)

	res, pres := self.Data[artifact_name]
	if !pres {
		return nil, false
	}

	// Caller did not specify a source - just give them a copy of
	// the complete artifact.
	if source_name == "" {
		return res, pres
	}

	// Caller asked for only a specific source in the artifact -
	// we therefore hand them back a copy with other sources
	// removed.
	new_artifact := proto.Clone(res).(*artifacts_proto.Artifact)
	new_artifact.Sources = nil

	for _, source := range res.Sources {
		if source.Name == source_name {
			new_artifact.Sources = append(new_artifact.Sources, source)
			return new_artifact, true
		}
	}

	// If we get here the source is not found in the artifact.
	return nil, false
}

func (self *Repository) Del(name string) {
	self.Lock()
	defer self.Unlock()

	delete(self.Data, name)
}

func (self *Repository) GetByPathPrefix(path string) []*artifacts_proto.Artifact {
	self.Lock()
	defer self.Unlock()

	name := strings.Replace(path, "/", ".", -1)

	result := []*artifacts_proto.Artifact{}
	for _, artifact := range self.Data {
		if strings.HasPrefix(artifact.Name, name) {
			result = append(result, artifact)
		}
	}

	return result
}

func (self *Repository) List() []string {
	self.Lock()
	defer self.Unlock()

	return self.list()
}

func (self *Repository) list() []string {
	result := make([]string, 0, len(self.Data))
	for k := range self.Data {
		result = append(result, k)
	}
	sort.Strings(result)
	return result
}

// Parse the query and determine if it requires any artifacts. If any
// artifacts are found, then recursivly determine their dependencies
// etc.
func (self *Repository) GetQueryDependencies(
	query string,
	depth int,
	dependency map[string]int) error {
	self.Lock()
	defer self.Unlock()

	return self.getQueryDependencies(query, depth, dependency)
}

// Called recursively to deterimine requirements at each level. If an
// artifact at a certain depth calls an artifact which is already used
// in a higher depth this is a cycle and we fail to compile.
func (self *Repository) getQueryDependencies(
	query string,
	depth int,
	dependency map[string]int) error {

	// For now this is really dumb - just search for something
	// that looks like an artifact.
	for _, hit := range artifact_in_query_regex.
		FindAllStringSubmatch(query, -1) {
		artifact_name := hit[1]
		dep, pres := self.Data[artifact_name]
		if !pres {
			return errors.New(
				fmt.Sprintf("Unknown artifact reference %s",
					artifact_name))
		}

		existing_depth, pres := dependency[hit[1]]
		if pres {
			if existing_depth < depth {
				return errors.New(
					fmt.Sprintf(
						"Cycle found while compiling %s", artifact_name))
			}
			continue
		}

		dependency[artifact_name] = depth

		// Now search the referred to artifact's query for its
		// own dependencies.
		err := self.getQueryDependencies(dep.Precondition, depth+1, dependency)
		if err != nil {
			return err
		}

		for _, source := range dep.Sources {
			err := self.getQueryDependencies(source.Precondition, depth+1, dependency)
			if err != nil {
				return err
			}

			err = self.getQueryDependencies(source.Query, depth+1, dependency)
			if err != nil {
				return err
			}
		}
	}

	return nil
}

// Attach additional artifacts to the request if needed to satisfy
// dependencies.
func (self *Repository) PopulateArtifactsVQLCollectorArgs(
	request *actions_proto.VQLCollectorArgs) error {
	dependencies := make(map[string]int)
	for _, query := range request.Query {
		err := self.GetQueryDependencies(query.VQL, 0, dependencies)
		if err != nil {
			return err
		}
	}

	for k := range dependencies {
		artifact, pres := self.Get(k)
		if pres {
			// Include any dependent tools.
			for _, required_tool := range artifact.RequiredTools {
				if !utils.InString(request.Tools, required_tool) {
					request.Tools = append(request.Tools, required_tool)
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
			// client.
			request.Artifacts = append(request.Artifacts,
				&artifacts_proto.Artifact{
					Name:       artifact.Name,
					Parameters: artifact.Parameters,
					Sources:    sources,
				})
		}
	}

	return nil
}

func NewRepository() *Repository {
	return &Repository{
		Data: make(map[string]*artifacts_proto.Artifact)}
}

func Parse(filename string) (*artifacts_proto.Artifact, error) {
	result := &artifacts_proto.Artifact{}
	data, err := ioutil.ReadFile(filename)
	if err != nil {
		return nil, errors.WithStack(err)
	}

	err = yaml.UnmarshalStrict(data, result)
	if err != nil {
		return nil, errors.WithStack(err)
	}
	result.Raw = string(data)

	return result, nil
}

func (self *Repository) Compile(artifact *artifacts_proto.Artifact,
	result *actions_proto.VQLCollectorArgs) error {
	for _, parameter := range artifact.Parameters {
		value := parameter.Default
		result.Env = append(result.Env, &actions_proto.VQLEnv{
			Key:   parameter.Name,
			Value: value,
		})
	}

	// Merge any tools we need.
	for _, required_tool := range artifact.RequiredTools {
		if !utils.InString(result.Tools, required_tool) {
			result.Tools = append(result.Tools, required_tool)
		}
	}

	return self.mergeSources(artifact, result, 0)
}

func (self *Repository) mergeSources(artifact *artifacts_proto.Artifact,
	result *actions_proto.VQLCollectorArgs,
	depth int) error {

	if depth > 10 {
		return errors.New("Recursive include detected.")
	}

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
			return errors.Wrap(
				err, fmt.Sprintf("While parsing source query"))
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

		if source.Precondition != "" {
			result.Query = append(result.Query, &actions_proto.VQLRequest{
				Name:        name,
				Description: description,
				VQL: fmt.Sprintf(
					"SELECT * FROM if(then=%s, condition=%s)",
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

	// Now process any includes.
	for _, include := range artifact.Includes {
		child, pres := self.Get(include)
		if pres {
			err := self.mergeSources(child, result, depth+1)
			if err != nil {
				return err
			}
		}
	}

	return nil
}

func escape_name(name string) string {
	return regexp.MustCompile("[^a-zA-Z0-9]").ReplaceAllString(name, "_")
}

type init_function func(*config_proto.Config) error

var init_registry []init_function

func GetGlobalRepository(config_obj *config_proto.Config) (*Repository, error) {
	mu.Lock()
	defer mu.Unlock()

	if global_repository != nil {
		return global_repository, nil
	}

	global_repository = NewRepository()
	for _, function := range init_registry {
		err := function(config_obj)
		if err != nil {
			return nil, err
		}
	}

	return global_repository, nil
}

func RegisterArtifactSources(fn init_function) {
	init_registry = append(init_registry, fn)
}

func splitQueryToQueries(query string) ([]string, error) {
	vqls, err := vfilter.MultiParse(query)
	if err != nil {
		return nil, err
	}

	scope := vql_subsystem.MakeScope()
	result := []string{}
	for _, vql := range vqls {
		result = append(result, vql.ToString(scope))
	}

	return result, nil
}

func compileArtifact(artifact *artifacts_proto.Artifact) error {
	for _, source := range artifact.Sources {
		if source.Queries == nil {
			// The Queries field contains the compiled queries -
			// removing any comments.
			queries, err := splitQueryToQueries(source.Query)
			if err != nil {
				return err
			}
			source.Queries = queries
		}
	}
	return nil
}
