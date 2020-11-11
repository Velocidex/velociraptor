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
package api

import (
	"archive/zip"
	"bytes"
	"errors"
	"fmt"
	"io/ioutil"
	"os"
	"path"
	"regexp"
	"sort"
	"strings"

	"github.com/sirupsen/logrus"
	context "golang.org/x/net/context"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"www.velocidex.com/golang/velociraptor/acls"
	actions_proto "www.velocidex.com/golang/velociraptor/actions/proto"
	api_proto "www.velocidex.com/golang/velociraptor/api/proto"
	artifacts_proto "www.velocidex.com/golang/velociraptor/artifacts/proto"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	"www.velocidex.com/golang/velociraptor/constants"
	file_store "www.velocidex.com/golang/velociraptor/file_store"
	flows_proto "www.velocidex.com/golang/velociraptor/flows/proto"
	"www.velocidex.com/golang/velociraptor/logging"
	"www.velocidex.com/golang/velociraptor/paths"
	"www.velocidex.com/golang/velociraptor/paths/artifacts"
	"www.velocidex.com/golang/velociraptor/services"
	users "www.velocidex.com/golang/velociraptor/users"
	"www.velocidex.com/golang/velociraptor/utils"
)

var (
	name_regex = regexp.MustCompile("(?sm)^(name: *)(.+)$")
)

const (
	default_artifact = `name: Custom.Artifact.Name
description: |
   This is the human readable description of the artifact.

# Can be CLIENT, CLIENT_EVENT, SERVER, SERVER_EVENT
type: CLIENT

parameters:
   - name: FirstParameter
     default: Default Value of first parameter

sources:
  - precondition:
      SELECT OS From info() where OS = 'windows' OR OS = 'linux' OR OS = 'darwin'

    query: |
      SELECT * FROM info()
      LIMIT 10
`
)

func getArtifactFile(
	config_obj *config_proto.Config,
	name string) (string, error) {

	manager, err := services.GetRepositoryManager()
	if err != nil {
		return "", err
	}

	repository, err := manager.GetGlobalRepository(config_obj)
	if err != nil {
		return "", err
	}

	artifact, pres := repository.Get(config_obj, name)
	if !pres {
		return default_artifact, nil
	}

	// This is hacky but necessary since we can not reserialize
	// the artifact - the yaml library is unable to properly round
	// trip the raw yaml.
	if !strings.HasPrefix(artifact.Name, constants.ARTIFACT_CUSTOM_NAME_PREFIX) {
		return ensureArtifactPrefix(artifact.Raw,
			constants.ARTIFACT_CUSTOM_NAME_PREFIX), nil
	}

	return artifact.Raw, nil
}

func ensureArtifactPrefix(definition, prefix string) string {
	return utils.ReplaceAllStringSubmatchFunc(
		name_regex, definition,
		func(matches []string) string {
			if !strings.HasPrefix(matches[2], prefix) {
				return matches[1] + prefix + matches[2]
			}
			return matches[1] + matches[2]
		})
}

func setArtifactFile(config_obj *config_proto.Config,
	in *api_proto.SetArtifactRequest,
	required_prefix string) (
	*artifacts_proto.Artifact, error) {

	manager, err := services.GetRepositoryManager()
	if err != nil {
		return nil, err
	}

	switch in.Op {
	case api_proto.SetArtifactRequest_DELETE:

		// First ensure that the artifact is correct.
		tmp_repository := manager.NewRepository()
		artifact_definition, err := tmp_repository.LoadYaml(
			in.Artifact, true /* validate */)
		if err != nil {
			return nil, err
		}

		if !strings.HasPrefix(artifact_definition.Name, required_prefix) {
			return nil, errors.New(
				"Modified or custom artifacts must start with '" +
					required_prefix + "'")
		}

		return artifact_definition, manager.DeleteArtifactFile(config_obj,
			artifact_definition.Name)

	case api_proto.SetArtifactRequest_SET:
		return manager.SetArtifactFile(
			config_obj, in.Artifact, required_prefix)
	}

	return nil, errors.New("Unknown op")
}

func getReportArtifacts(
	config_obj *config_proto.Config,
	report_type string,
	number_of_results uint64) (
	*artifacts_proto.ArtifactDescriptors, error) {

	if number_of_results == 0 {
		number_of_results = 100
	}

	manager, err := services.GetRepositoryManager()
	if err != nil {
		return nil, err
	}
	repository, err := manager.GetGlobalRepository(config_obj)
	if err != nil {
		return nil, err
	}

	result := &artifacts_proto.ArtifactDescriptors{}
	for _, name := range repository.List() {
		artifact, pres := repository.Get(config_obj, name)
		if pres {
			for _, report := range artifact.Reports {
				if report.Type == report_type {
					result.Items = append(
						result.Items, artifact)
				}
			}
		}

		if len(result.Items) >= int(number_of_results) {
			break
		}
	}

	return result, nil
}

func searchArtifact(
	config_obj *config_proto.Config,
	terms []string,
	artifact_type string,
	number_of_results uint64) (
	*artifacts_proto.ArtifactDescriptors, error) {

	if config_obj.GUI == nil {
		return nil, errors.New("GUI not configured")
	}

	name_filter_regexp := config_obj.GUI.ArtifactSearchFilter
	if name_filter_regexp == "" {
		name_filter_regexp = "."
	}
	name_filter := regexp.MustCompile(name_filter_regexp)

	artifact_type = strings.ToLower(artifact_type)

	if number_of_results == 0 {
		number_of_results = 100
	}

	result := &artifacts_proto.ArtifactDescriptors{}
	regexes := []*regexp.Regexp{}
	for _, term := range terms {
		if len(term) <= 2 {
			continue
		}

		re, err := regexp.Compile("(?i)" + term)
		if err == nil {
			regexes = append(regexes, re)
		}
	}

	if len(regexes) == 0 {
		return result, nil
	}

	matcher := func(text string, regexes []*regexp.Regexp) bool {
		for _, re := range regexes {
			if re.FindString(text) == "" {
				return false
			}
		}
		return true
	}

	manager, err := services.GetRepositoryManager()
	if err != nil {
		return nil, err
	}
	repository, err := manager.GetGlobalRepository(config_obj)
	if err != nil {
		return nil, err
	}

	for _, name := range repository.List() {
		if name_filter.FindString(name) == "" {
			continue
		}

		artifact, pres := repository.Get(config_obj, name)
		if pres {
			// Skip non matching types
			if artifact_type != "" &&
				artifact.Type != artifact_type {
				continue
			}

			if matcher(artifact.Description, regexes) ||
				matcher(artifact.Name, regexes) {
				result.Items = append(result.Items, artifact)
			}
		}

		if len(result.Items) >= int(number_of_results) {
			break
		}
	}

	return result, nil
}

func (self *ApiServer) LoadArtifactPack(
	ctx context.Context,
	in *api_proto.VFSFileBuffer) (
	*api_proto.LoadArtifactPackResponse, error) {

	user_name := GetGRPCUserInfo(self.config, ctx).Name
	user_record, err := users.GetUser(self.config, user_name)
	if err != nil {
		return nil, err
	}

	permissions := acls.SERVER_ARTIFACT_WRITER
	perm, err := acls.CheckAccess(self.config, user_record.Name, permissions)
	if !perm || err != nil {
		return nil, status.Error(codes.PermissionDenied,
			"User is not allowed to upload artifact packs.")
	}

	prefix := constants.ARTIFACT_PACK_NAME_PREFIX

	result := &api_proto.LoadArtifactPackResponse{}
	buffer := bytes.NewReader(in.Data)
	zip_reader, err := zip.NewReader(buffer, int64(len(in.Data)))
	if err != nil {
		return nil, err
	}

	for _, file := range zip_reader.File {
		if strings.HasSuffix(file.Name, ".yaml") {
			fd, err := file.Open()
			if err != nil {
				continue
			}

			data, err := ioutil.ReadAll(fd)
			fd.Close()

			if err != nil {
				continue
			}

			// Make sure the artifact is written into the
			// Packs part to prevent clashes with built in
			// names.
			artifact_definition := ensureArtifactPrefix(
				string(data), prefix)

			request := &api_proto.SetArtifactRequest{
				Op:       api_proto.SetArtifactRequest_SET,
				Artifact: artifact_definition,
			}

			definition, err := setArtifactFile(self.config, request, prefix)
			if err == nil {
				logging.GetLogger(self.config, &logging.Audit).
					WithFields(logrus.Fields{
						"user":     user_name,
						"artifact": definition.Name,
						"details": fmt.Sprintf(
							"%v", request.Artifact),
					}).Info("LoadArtifactPack")

				result.SuccessfulArtifacts = append(result.SuccessfulArtifacts,
					definition.Name)
			} else {
				result.Errors = append(result.Errors, &api_proto.LoadArtifactError{
					Filename: file.Name,
					Error:    err.Error(),
				})
			}
		}
	}

	return result, nil
}

func (self *ApiServer) ListAvailableEventResults(
	ctx context.Context,
	in *api_proto.ListAvailableEventResultsRequest) (
	*api_proto.ListAvailableEventResultsResponse, error) {

	user_name := GetGRPCUserInfo(self.config, ctx).Name
	user_record, err := users.GetUser(self.config, user_name)
	if err != nil {
		return nil, err
	}

	permissions := acls.READ_RESULTS
	perm, err := acls.CheckAccess(self.config, user_record.Name, permissions)
	if !perm || err != nil {
		return nil, status.Error(codes.PermissionDenied,
			"User is not allowed to view results.")
	}

	manager, err := services.GetRepositoryManager()
	if err != nil {
		return nil, err
	}

	repository, err := manager.GetGlobalRepository(self.config)
	if err != nil {
		return nil, err
	}

	path_manager := artifacts.NewMonitoringArtifactPathManager(in.ClientId)
	file_store_factory := file_store.GetFileStore(self.config)

	seen := make(map[string]*api_proto.AvailableEvent)
	err = file_store_factory.Walk(path_manager.Path(),
		func(full_path string, info os.FileInfo, err error) error {
			if !info.IsDir() && info.Size() > 0 {
				relative_path := strings.TrimPrefix(full_path, path_manager.Path())
				artifact_name := strings.TrimLeft(path.Dir(relative_path), "/")
				date_name := path.Base(relative_path)
				timestamp := paths.DayNameToTimestamp(date_name)

				if timestamp != 0 {
					event, pres := seen[artifact_name]
					if !pres {
						event = &api_proto.AvailableEvent{
							Artifact: artifact_name,
						}

						artifact, pres := repository.Get(
							self.config, artifact_name)
						if pres {
							event.Definition = artifact
						}
					}
					event.Timestamps = append(event.Timestamps,
						int32(timestamp))
					seen[artifact_name] = event
				}
			}
			return nil
		})
	if err != nil {
		return nil, err
	}

	result := &api_proto.ListAvailableEventResultsResponse{}
	for _, item := range seen {
		result.Logs = append(result.Logs, item)
	}

	sort.Slice(result.Logs, func(i, j int) bool {
		return result.Logs[i].Artifact < result.Logs[j].Artifact
	})

	return result, nil
}

// MakeCollectorRequest is a convenience function for creating
// flows_proto.ArtifactCollectorArgs protobufs.
func MakeCollectorRequest(
	client_id string, artifact_name string,
	parameters ...string) *flows_proto.ArtifactCollectorArgs {
	result := &flows_proto.ArtifactCollectorArgs{
		ClientId:   client_id,
		Artifacts:  []string{artifact_name},
		Parameters: &flows_proto.ArtifactParameters{},
	}

	if len(parameters)%2 != 0 {
		parameters = parameters[:len(parameters)-len(parameters)%2]
	}

	if parameters != nil {
		for i := 0; i < len(parameters); {
			k := parameters[i]
			i++
			v := parameters[i]
			i++
			result.Parameters.Env = append(result.Parameters.Env,
				&actions_proto.VQLEnv{
					Key: k, Value: v,
				})
		}
	}

	return result
}
