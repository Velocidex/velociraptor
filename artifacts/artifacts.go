package artifacts

import (
	"fmt"
	errors "github.com/pkg/errors"
	"gopkg.in/yaml.v2"
	"io/ioutil"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	actions_proto "www.velocidex.com/golang/velociraptor/actions/proto"
	artifacts_proto "www.velocidex.com/golang/velociraptor/artifacts/proto"
	//	debug "www.velocidex.com/golang/velociraptor/testing"
)

// Holds multiple artifact definitions.
type Repository struct {
	data map[string]*artifacts_proto.Artifact
}

func (self *Repository) LoadDirectory(dirname string) error {
	return filepath.Walk(dirname,
		func(path string, info os.FileInfo, err error) error {
			if err != nil {
				return err
			}

			if !info.IsDir() && strings.HasSuffix(info.Name(), ".yaml") {
				artifact, err := Parse(path)
				if err != nil {
					return err
				}

				self.data[artifact.Name] = artifact
			}
			return nil
		})
}

func (self *Repository) Get(name string) (*artifacts_proto.Artifact, bool) {
	res, pres := self.data[name]
	return res, pres
}

func (self *Repository) List() []string {
	result := []string{}
	for k, _ := range self.data {
		result = append(result, k)
	}
	return result
}

func NewRepository() *Repository {
	return &Repository{make(map[string]*artifacts_proto.Artifact)}
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
