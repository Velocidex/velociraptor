/*
   Velociraptor - Dig Deeper
   Copyright (C) 2019-2022 Rapid7 Inc.

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
	"regexp"
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
	flows_proto "www.velocidex.com/golang/velociraptor/flows/proto"
	"www.velocidex.com/golang/velociraptor/logging"
	"www.velocidex.com/golang/velociraptor/services"
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

	manager, err := services.GetRepositoryManager(config_obj)
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
	if artifact.BuiltIn {
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

func setArtifactFile(config_obj *config_proto.Config, principal string,
	in *api_proto.SetArtifactRequest,
	required_prefix string) (
	*artifacts_proto.Artifact, error) {

	manager, err := services.GetRepositoryManager(config_obj)
	if err != nil {
		return nil, err
	}

	switch in.Op {
	case api_proto.SetArtifactRequest_DELETE:

		// First ensure that the artifact is correct.
		tmp_repository := manager.NewRepository()
		artifact_definition, err := tmp_repository.LoadYaml(
			in.Artifact, true /* validate */, false /* built_in */)
		if err != nil {
			return nil, err
		}

		if !strings.HasPrefix(artifact_definition.Name, required_prefix) {
			return nil, errors.New(
				"Modified or custom artifact names must start with '" +
					required_prefix + "'")
		}

		return artifact_definition, manager.DeleteArtifactFile(config_obj,
			principal, artifact_definition.Name)

	case api_proto.SetArtifactRequest_SET:
		return manager.SetArtifactFile(
			config_obj, principal, in.Artifact, required_prefix)
	}

	return nil, errors.New("Unknown op")
}

func getReportArtifacts(
	ctx context.Context,
	config_obj *config_proto.Config,
	report_type string,
	number_of_results uint64) (
	*artifacts_proto.ArtifactDescriptors, error) {

	if number_of_results == 0 {
		number_of_results = 100
	}

	manager, err := services.GetRepositoryManager(config_obj)
	if err != nil {
		return nil, err
	}
	repository, err := manager.GetGlobalRepository(config_obj)
	if err != nil {
		return nil, err
	}

	result := &artifacts_proto.ArtifactDescriptors{}
	names, err := repository.List(ctx, config_obj)
	if err != nil {
		return nil, err
	}
	for _, name := range names {
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
	ctx context.Context,
	config_obj *config_proto.Config,
	terms []string,
	artifact_type string,
	number_of_results uint64, fields *api_proto.FieldSelector) (
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
		number_of_results = 1000
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

	manager, err := services.GetRepositoryManager(config_obj)
	if err != nil {
		return nil, err
	}
	repository, err := manager.GetGlobalRepository(config_obj)
	if err != nil {
		return nil, err
	}

	names, err := repository.List(ctx, config_obj)
	if err != nil {
		return nil, err
	}

	for _, name := range names {
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
				if fields == nil {
					result.Items = append(result.Items, artifact)
				} else {
					// Send back minimal information about the
					// artifacts
					new_item := &artifacts_proto.Artifact{}
					if fields.Name {
						new_item.Name = artifact.Name
						new_item.BuiltIn = artifact.BuiltIn
					}

					if fields.Description {
						new_item.Description = artifact.Description
					}
					if fields.Type {
						new_item.Type = artifact.Type
					}

					result.Items = append(result.Items, new_item)
				}
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

	users_manager := services.GetUserManager()
	user_record, org_config_obj, err := users_manager.GetUserFromContext(ctx)
	if err != nil {
		return nil, err
	}

	user_name := user_record.Name
	permissions := acls.SERVER_ARTIFACT_WRITER
	perm, err := acls.CheckAccess(org_config_obj, user_record.Name, permissions)
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

			definition, err := setArtifactFile(
				org_config_obj, user_name, request, prefix)
			if err == nil {
				logging.GetLogger(org_config_obj, &logging.Audit).
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

// MakeCollectorRequest is a convenience function for creating
// flows_proto.ArtifactCollectorArgs protobufs.
func MakeCollectorRequest(
	client_id string, artifact_name string,
	parameters ...string) *flows_proto.ArtifactCollectorArgs {
	result := &flows_proto.ArtifactCollectorArgs{
		ClientId:  client_id,
		Artifacts: []string{artifact_name},
		Specs: []*flows_proto.ArtifactSpec{
			{
				Artifact:   artifact_name,
				Parameters: &flows_proto.ArtifactParameters{},
			},
		},
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
			result.Specs[0].Parameters.Env = append(result.Specs[0].Parameters.Env,
				&actions_proto.VQLEnv{
					Key: k, Value: v,
				})
		}
	}

	return result
}
