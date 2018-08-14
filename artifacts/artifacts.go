package artifacts

import (
	"fmt"
	errors "github.com/pkg/errors"
	"gopkg.in/yaml.v2"
	"io/ioutil"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	actions_proto "www.velocidex.com/golang/velociraptor/actions/proto"
	artifacts_proto "www.velocidex.com/golang/velociraptor/artifacts/proto"
	config "www.velocidex.com/golang/velociraptor/config"
	logging "www.velocidex.com/golang/velociraptor/logging"
	utils "www.velocidex.com/golang/velociraptor/utils"
)

var (
	artifact_in_query_regex = regexp.MustCompile("Artifact\\.([^\\s\\(]+)\\(")
	global_repository       = NewRepository()
	mu                      sync.Mutex
)

// Holds multiple artifact definitions.
type Repository struct {
	data        map[string]*artifacts_proto.Artifact
	loaded_dirs []string
}

func (self *Repository) LoadDirectory(dirname string) error {
	if utils.InString(&self.loaded_dirs, dirname) {
		return nil
	}
	self.loaded_dirs = append(self.loaded_dirs, dirname)
	return filepath.Walk(dirname,
		func(file_path string, info os.FileInfo, err error) error {
			if err != nil {
				return errors.WithStack(err)
			}

			if !info.IsDir() && strings.HasSuffix(info.Name(), ".yaml") {
				artifact, err := Parse(file_path)
				if err != nil {
					return errors.Wrap(err, path.Join(dirname, info.Name()))
				}

				self.data[artifact.Name] = artifact
			}
			return nil
		})
}

func (self *Repository) LoadYaml(data string) error {
	artifact := &artifacts_proto.Artifact{}
	err := yaml.UnmarshalStrict([]byte(data), artifact)
	if err != nil {
		return errors.WithStack(err)
	}
	self.data[artifact.Name] = artifact
	return nil
}

func (self *Repository) Get(name string) (*artifacts_proto.Artifact, bool) {
	res, pres := self.data[name]
	return res, pres
}

func (self *Repository) Set(artifact *artifacts_proto.Artifact) {
	self.data[artifact.Name] = artifact
}

func (self *Repository) List() []string {
	result := []string{}
	for k, _ := range self.data {
		result = append(result, k)
	}
	return result
}

// Parse the query and determine if it requires any artifacts. If any
// artifacts are found, then recursive determine their dependencies
// etc.
func (self *Repository) GetQueryDependencies(query string) []string {
	// For now this is really dumb - just search for something
	// that looks like an artifact.
	result := []string{}
	for _, hit := range artifact_in_query_regex.FindAllStringSubmatch(
		query, -1) {
		_, pres := self.data[hit[1]]
		if pres {
			result = append(result, hit[1])
		}
	}

	return result
}

func (self *Repository) PopulateArtifactsVQLCollectorArgs(
	request *actions_proto.VQLCollectorArgs) {
	dependencies := make(map[string]bool)
	for _, query := range request.Query {
		for _, dep := range self.GetQueryDependencies(query.VQL) {
			dependencies[dep] = true
		}
	}
	for k, _ := range dependencies {
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
}

func NewRepository() *Repository {
	return &Repository{
		data: make(map[string]*artifacts_proto.Artifact)}
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

	return result, nil
}

func Compile(artifact *artifacts_proto.Artifact,
	result *actions_proto.VQLCollectorArgs) error {
	for _, parameter := range artifact.Parameters {
		value := parameter.Default
		result.Env = append(result.Env, &actions_proto.VQLEnv{
			Key:   parameter.Name,
			Value: value,
		})
	}

	for idx, source := range artifact.Sources {
		prefix := fmt.Sprintf("%s_%d", escape_name(artifact.Name), idx)
		source_result := ""
		source_precondition := "precondition_" + prefix
		result.Query = append(result.Query, &actions_proto.VQLRequest{
			VQL: "LET " + source_precondition + " = " +
				source.Precondition,
		})

		queries := []string{}
		for idx2, query := range source.Queries {
			query_name := fmt.Sprintf("%s_%d", prefix, idx2)
			if strings.HasPrefix(query, "LET") {
				result.Query = append(result.Query,
					&actions_proto.VQLRequest{
						VQL: query,
					})
			} else {
				result.Query = append(result.Query,
					&actions_proto.VQLRequest{
						VQL: "LET " + query_name +
							" = " + query,
					})
				queries = append(queries, query_name)
			}
			source_result = query_name
		}

		if len(queries) > 1 {
			result.Query = append(result.Query, &actions_proto.VQLRequest{
				VQL: fmt.Sprintf(
					"LET "+source_result+" = SELECT * FROM chain(%s)",
					strings.Join(queries, ", ")),
			})
			source_result = "Result_" + prefix
		}

		result.Query = append(result.Query, &actions_proto.VQLRequest{
			VQL: fmt.Sprintf(
				"SELECT * FROM if(query=%s, condition=%s)",
				source_result, source_precondition),
		})
	}

	return nil
}

func escape_name(name string) string {
	return regexp.MustCompile("[^a-zA-Z0-9]").ReplaceAllString(name, "_")
}

func GetGlobalRepository(config_obj *config.Config) (*Repository, error) {
	mu.Lock()
	defer mu.Unlock()

	logger := logging.NewLogger(config_obj)
	if config_obj.Frontend.ArtifactsPath != "" {
		logger.Info("Loading artifacts from %s", config_obj.Frontend.ArtifactsPath)
		err := global_repository.LoadDirectory(config_obj.Frontend.ArtifactsPath)
		switch errors.Cause(err).(type) {
		// PathError is not fatal - it means we just cant load the directory.
		case *os.PathError:
			logger.Info("Unable to load artifacts from directory "+
				"%s (skipping): %v", config_obj.Frontend.ArtifactsPath, err)
		case nil:
			break
		default:
			// Other errors are fatal - they mean we cant
			// parse the artifacts themselves.
			return nil, err
		}
	}
	return global_repository, nil
}
