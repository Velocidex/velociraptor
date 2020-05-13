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
	"errors"
	"os"
	"path"
	"regexp"
	"strings"

	context "golang.org/x/net/context"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"www.velocidex.com/golang/velociraptor/acls"
	actions_proto "www.velocidex.com/golang/velociraptor/actions/proto"
	api_proto "www.velocidex.com/golang/velociraptor/api/proto"
	"www.velocidex.com/golang/velociraptor/artifacts"
	artifacts_proto "www.velocidex.com/golang/velociraptor/artifacts/proto"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	file_store "www.velocidex.com/golang/velociraptor/file_store"
	flows_proto "www.velocidex.com/golang/velociraptor/flows/proto"
	"www.velocidex.com/golang/velociraptor/paths"
	"www.velocidex.com/golang/velociraptor/result_sets"
	users "www.velocidex.com/golang/velociraptor/users"
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

# Reports can be MONITORING_DAILY, CLIENT, SERVER_EVENT
reports:
  - type: CLIENT
    template: |
      {{ .Description }}

`
)

func getArtifactFile(
	config_obj *config_proto.Config,
	name string) (string, error) {

	repository, err := artifacts.GetGlobalRepository(config_obj)
	if err != nil {
		return "", err
	}

	artifact, pres := repository.Get(name)
	if !pres {
		return default_artifact, nil
	}

	// This is hacky but necessary since we can not reserialize
	// the artifact - the yaml library is unable to properly round
	// trip the raw yaml.
	if !strings.HasPrefix(artifact.Name, "Custom.") {
		regex, err := regexp.Compile(
			"(?s)(?m)^name:\\s*" + artifact.Name + "$")
		if err != nil {
			return default_artifact, err
		}

		result := regex.ReplaceAllString(
			artifact.Raw, "name: Custom."+artifact.Name)
		return result, nil
	}

	return artifact.Raw, nil
}

func setArtifactFile(config_obj *config_proto.Config,
	in *api_proto.SetArtifactRequest) (
	*artifacts_proto.Artifact, error) {

	// First ensure that the artifact is correct.
	tmp_repository := artifacts.NewRepository()
	artifact_definition, err := tmp_repository.LoadYaml(
		in.Artifact, true /* validate */)
	if err != nil {
		return nil, err
	}

	if !strings.HasPrefix(artifact_definition.Name, "Custom.") {
		return nil, errors.New(
			"Modified or custom artifacts must start with 'Custom'")
	}

	file_store_factory := file_store.GetFileStore(config_obj)
	vfs_path := paths.GetArtifactDefintionPath(artifact_definition.Name)

	// Load the new artifact into the global repo so it is
	// immediately available.
	global_repository, err := artifacts.GetGlobalRepository(config_obj)
	if err != nil {
		return nil, err
	}

	switch in.Op {

	case api_proto.SetArtifactRequest_DELETE:
		global_repository.Del(artifact_definition.Name)
		err = file_store_factory.Delete(vfs_path)
		return artifact_definition, err

	case api_proto.SetArtifactRequest_SET:
		// Now write it into the filestore.
		fd, err := file_store_factory.WriteFile(vfs_path)
		if err != nil {
			return nil, err
		}
		defer fd.Close()

		// We want to completely replace the content of the file.
		err = fd.Truncate()
		if err != nil {
			return nil, err
		}

		_, err = fd.Write([]byte(in.Artifact))
		if err != nil {
			return nil, err
		}

		// Load the artifact into the currently running repository.
		// Artifact is already valid - no need to revalidate it again.
		_, err = global_repository.LoadYaml(in.Artifact, false /* validate */)
		return artifact_definition, err
	}

	return nil, errors.New("Unknown op")
}

func searchArtifact(
	config_obj *config_proto.Config,
	terms []string,
	artifact_type string,
	number_of_results uint64) (
	*artifacts_proto.ArtifactDescriptors, error) {

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

	repository, err := artifacts.GetGlobalRepository(config_obj)
	if err != nil {
		return nil, err
	}

	for _, name := range repository.List() {
		artifact, pres := repository.Get(name)
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

	path_manager := result_sets.NewMonitoringArtifactPathManager(in.ClientId)
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
