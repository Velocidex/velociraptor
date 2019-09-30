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

	"github.com/Velocidex/yaml"
	errors "github.com/pkg/errors"
	actions_proto "www.velocidex.com/golang/velociraptor/actions/proto"
	artifacts_proto "www.velocidex.com/golang/velociraptor/artifacts/proto"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	utils "www.velocidex.com/golang/velociraptor/utils"
	"www.velocidex.com/golang/vfilter"
)

var (
	artifact_in_query_regex = regexp.MustCompile(`Artifact\.([^\s\(]+)\(`)
	global_repository       *Repository
	mu                      sync.Mutex
)

// Holds multiple artifact definitions.
type Repository struct {
	Data        map[string]*artifacts_proto.Artifact
	loaded_dirs []string
}

func (self *Repository) LoadDirectory(dirname string) (*int, error) {
	count := 0
	if utils.InString(&self.loaded_dirs, dirname) {
		return &count, nil
	}
	dirname = filepath.Clean(dirname)
	self.loaded_dirs = append(self.loaded_dirs, dirname)
	return &count, filepath.Walk(dirname,
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
}

func (self *Repository) LoadYaml(data string, validate bool) (
	*artifacts_proto.Artifact, error) {
	artifact := &artifacts_proto.Artifact{}
	err := yaml.UnmarshalStrict([]byte(data), artifact)
	if err != nil {
		return nil, errors.WithStack(err)
	}
	artifact.Raw = data

	// Validate the artifact.
	for _, report := range artifact.Reports {
		report.Type = strings.ToLower(report.Type)
		switch report.Type {
		case "monitoring_daily", "server_event", "client", "internal", "hunt":
			continue
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

	// Validate the artifact contains syntactically correct VQL.
	if validate {
		for _, source := range artifact.Sources {
			if source.Precondition != "" {
				_, err := vfilter.Parse(source.Precondition)
				if err != nil {
					return nil, err
				}
			}

			for _, query := range source.Queries {
				_, err := vfilter.Parse(query)
				if err != nil {
					return nil, err
				}
			}
		}
	}

	self.Data[artifact.Name] = artifact
	return artifact, nil
}

func (self *Repository) Get(name string) (*artifacts_proto.Artifact, bool) {
	res, pres := self.Data[name]
	return res, pres
}

func (self *Repository) Del(name string) {
	delete(self.Data, name)
}

func (self *Repository) GetByPathPrefix(path string) []*artifacts_proto.Artifact {
	name := strings.Replace(path, "/", ".", -1)

	result := []*artifacts_proto.Artifact{}
	for _, artifact := range self.Data {
		if strings.HasPrefix(artifact.Name, name) {
			result = append(result, artifact)
		}
	}

	return result
}

func (self *Repository) Set(artifact *artifacts_proto.Artifact) {
	self.Data[artifact.Name] = artifact
}

func (self *Repository) List() []string {
	result := []string{}
	for k := range self.Data {
		result = append(result, k)
	}
	sort.Strings(result)
	return result
}

// Parse the query and determine if it requires any artifacts. If any
// artifacts are found, then recursivly determine their dependencies
// etc.

// Called recursively to deterimine requirements at each level. If an
// artifact at a certain depth calls an artifact which is already used
// in a higher depth this is a cycle and we fail to compile.
func (self *Repository) GetQueryDependencies(
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
		for _, source := range dep.Sources {
			for _, query := range source.Queries {
				err := self.GetQueryDependencies(query, depth+1, dependency)
				if err != nil {
					return err
				}
			}
		}
	}

	return nil
}

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
			// Deliberately make a copy of the artifact -
			// we do not want to give away metadata to the
			// client.
			request.Artifacts = append(request.Artifacts,
				&artifacts_proto.Artifact{
					Name:       artifact.Name,
					Parameters: artifact.Parameters,
					Sources:    artifact.Sources,
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

	return self.mergeSources(artifact, result, 0)
}

func (self *Repository) mergeSources(artifact *artifacts_proto.Artifact,
	result *actions_proto.VQLCollectorArgs,
	depth int) error {

	if depth > 10 {
		return errors.New("Recursive include detected.")
	}

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
		for idx2, query := range source.Queries {
			// Verify the query's syntax.
			vql, err := vfilter.Parse(query)
			if err != nil {
				return errors.Wrap(
					err, fmt.Sprintf(
						"While parsing source query %d",
						idx2))
			}

			query_name := fmt.Sprintf("%s_%d", prefix, idx2)
			if idx2 < len(source.Queries)-1 {
				if vql.Let == "" {
					return errors.New(
						"Invalid artifact: All Queries in a source " +
							"must be LET queries, except for the " +
							"final one.")
				}
				result.Query = append(result.Query,
					&actions_proto.VQLRequest{
						VQL: query,
					})
			} else {
				if vql.Let != "" {
					return errors.New(
						"Invalid artifact: All Queries in a source " +
							"must be LET queries, except for the " +
							"final one.")
				}

				result.Query = append(result.Query,
					&actions_proto.VQLRequest{
						VQL: "LET " + query_name +
							" = " + query,
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
