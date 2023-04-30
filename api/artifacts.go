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
	"bytes"
	"io/ioutil"
	"regexp"
	"strings"

	"github.com/Velocidex/ordereddict"
	errors "github.com/go-errors/errors"
	context "golang.org/x/net/context"
	"www.velocidex.com/golang/velociraptor/acls"
	actions_proto "www.velocidex.com/golang/velociraptor/actions/proto"
	api_proto "www.velocidex.com/golang/velociraptor/api/proto"
	artifacts_proto "www.velocidex.com/golang/velociraptor/artifacts/proto"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	"www.velocidex.com/golang/velociraptor/constants"
	"www.velocidex.com/golang/velociraptor/file_store"
	"www.velocidex.com/golang/velociraptor/file_store/api"
	"www.velocidex.com/golang/velociraptor/file_store/path_specs"
	flows_proto "www.velocidex.com/golang/velociraptor/flows/proto"
	"www.velocidex.com/golang/velociraptor/paths"
	"www.velocidex.com/golang/velociraptor/services"
	"www.velocidex.com/golang/velociraptor/third_party/zip"
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
	ctx context.Context, config_obj *config_proto.Config,
	name string) (string, error) {

	manager, err := services.GetRepositoryManager(config_obj)
	if err != nil {
		return "", err
	}

	repository, err := manager.GetGlobalRepository(config_obj)
	if err != nil {
		return "", err
	}

	artifact, pres := repository.Get(ctx, config_obj, name)
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

func setArtifactFile(
	ctx context.Context, config_obj *config_proto.Config, principal string,
	in *api_proto.SetArtifactRequest, required_prefix string) (
	*artifacts_proto.Artifact, error) {

	manager, err := services.GetRepositoryManager(config_obj)
	if err != nil {
		return nil, Status(config_obj.Verbose, err)
	}

	switch in.Op {
	case api_proto.SetArtifactRequest_DELETE:

		// First ensure that the artifact is correct.
		tmp_repository := manager.NewRepository()
		artifact_definition, err := tmp_repository.LoadYaml(
			in.Artifact, services.ArtifactOptions{
				ValidateArtifact: true,
			})
		if err != nil {
			return nil, Status(config_obj.Verbose, err)
		}

		if !strings.HasPrefix(artifact_definition.Name, required_prefix) {
			return nil, InvalidStatus(
				"Modified or custom artifact names must start with '" +
					required_prefix + "'")
		}

		return artifact_definition, manager.DeleteArtifactFile(ctx, config_obj,
			principal, artifact_definition.Name)

	case api_proto.SetArtifactRequest_CHECK:
		tmp_repository := manager.NewRepository()
		return tmp_repository.LoadYaml(
			in.Artifact, services.ArtifactOptions{
				ValidateArtifact: true,
			})

	case api_proto.SetArtifactRequest_SET:
		return manager.SetArtifactFile(ctx,
			config_obj, principal, in.Artifact, required_prefix)
	}

	return nil, InvalidStatus("Unknown op")
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
		return nil, Status(config_obj.Verbose, err)
	}
	repository, err := manager.GetGlobalRepository(config_obj)
	if err != nil {
		return nil, Status(config_obj.Verbose, err)
	}

	result := &artifacts_proto.ArtifactDescriptors{}
	names, err := repository.List(ctx, config_obj)
	if err != nil {
		return nil, Status(config_obj.Verbose, err)
	}
	for _, name := range names {
		artifact, pres := repository.Get(ctx, config_obj, name)
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

type matchPlan struct {
	// These must match against the artifact name
	name_regex []*regexp.Regexp

	// These must match against the artifact preconditions
	precondition_regex []*regexp.Regexp

	tool_regex []*regexp.Regexp

	// Acceptable types
	types []string

	builtin *bool
}

func (self *matchPlan) matchDescOrName(artifact *artifacts_proto.Artifact) bool {
	// If no name regexp are specified we do not reject based on name.
	if len(self.name_regex) == 0 {
		return true
	}

	// All regex must match the same artifact - either in the name or
	// description.
	matches := 0
	for _, re := range self.name_regex {
		if re.MatchString(artifact.Name) {
			matches++
		} else if re.MatchString(artifact.Description) {
			matches++
		}
	}
	return matches == len(self.name_regex)
}

func (self *matchPlan) matchTool(artifact *artifacts_proto.Artifact) bool {
	if len(self.tool_regex) == 0 {
		return true
	}

	if len(artifact.Tools) == 0 {
		return false
	}

	for _, re := range self.tool_regex {
		for _, t := range artifact.Tools {
			if re.MatchString(t.Name) {
				return true
			}
		}
	}
	return false
}

// Preconditions can exist at the artifact level or at each source.
func (self *matchPlan) matchPreconditions(artifact *artifacts_proto.Artifact) bool {
	if len(self.precondition_regex) == 0 {
		return true
	}

	for _, re := range self.precondition_regex {
		if artifact.Precondition != "" &&
			re.MatchString(artifact.Precondition) {
			return true
		}
		for _, s := range artifact.Sources {
			if s.Precondition != "" &&
				re.MatchString(s.Precondition) {
				return true
			}
		}
	}
	return false
}

func (self *matchPlan) matchBuiltin(artifact *artifacts_proto.Artifact) bool {
	if self.builtin == nil {
		return true
	}

	if *self.builtin {
		return artifact.BuiltIn
	}
	return !artifact.BuiltIn
}

func (self *matchPlan) matchType(artifact *artifacts_proto.Artifact) bool {
	if len(self.types) > 0 {
		for _, t := range self.types {
			if strings.ToLower(artifact.Type) == t {
				return true
			}
		}
		return false
	}
	return true
}

// All conditions must match
func (self *matchPlan) matchArtifact(artifact *artifacts_proto.Artifact) bool {
	if !self.matchType(artifact) {
		return false
	}

	if !self.matchDescOrName(artifact) {
		return false
	}

	if !self.matchPreconditions(artifact) {
		return false
	}

	if !self.matchBuiltin(artifact) {
		return false
	}

	if !self.matchTool(artifact) {
		return false
	}

	return true
}

func prepareMatchPlan(search string) *matchPlan {
	result := &matchPlan{}
	// Tokenise the search expression into search terms:
	for _, token := range strings.Split(search, " ") {
		if token == "" {
			continue
		}

		parts := strings.SplitN(token, ":", 2)
		if len(parts) == 2 {
			verb := parts[0]
			term := parts[1]
			switch verb {
			case "type":
				result.types = append(result.types,
					strings.ToLower(term))
				continue

			case "precondition":
				re, err := regexp.Compile("(?i)" + term)
				if err == nil {
					result.precondition_regex = append(
						result.precondition_regex, re)
				}
				continue

			case "tool":
				re, err := regexp.Compile("(?i)" + term)
				if err == nil {
					result.tool_regex = append(
						result.tool_regex, re)
				}
				continue

			case "builtin":
				value := false
				if term == "yes" {
					value = true
				}
				result.builtin = &value
				continue
			}
		}
		re, err := regexp.Compile("(?i)" + token)
		if err == nil {
			result.name_regex = append(
				result.name_regex, re)
		}
	}
	return result
}

func searchArtifact(
	ctx context.Context,
	config_obj *config_proto.Config,
	search_term string,
	artifact_type string,
	number_of_results uint64, fields *api_proto.FieldSelector) (
	*artifacts_proto.ArtifactDescriptors, error) {

	if config_obj.GUI == nil {
		return nil, InvalidStatus("GUI not configured")
	}

	matcher := prepareMatchPlan(search_term)
	if artifact_type != "" {
		matcher.types = append(matcher.types, strings.ToLower(artifact_type))
	}

	if number_of_results == 0 {
		number_of_results = 1000
	}

	result := &artifacts_proto.ArtifactDescriptors{}
	manager, err := services.GetRepositoryManager(config_obj)
	if err != nil {
		return nil, Status(config_obj.Verbose, err)
	}
	repository, err := manager.GetGlobalRepository(config_obj)
	if err != nil {
		return nil, Status(config_obj.Verbose, err)
	}

	names, err := repository.List(ctx, config_obj)
	if err != nil {
		return nil, Status(config_obj.Verbose, err)
	}

	for _, name := range names {
		artifact, pres := repository.Get(ctx, config_obj, name)
		if !pres {
			continue
		}

		if matcher.matchArtifact(artifact) {
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

		if len(result.Items) >= int(number_of_results) {
			break
		}
	}

	return result, nil
}

func (self *ApiServer) LoadArtifactPack(
	ctx context.Context,
	in *api_proto.LoadArtifactPackRequest) (
	*api_proto.LoadArtifactPackResponse, error) {

	users_manager := services.GetUserManager()
	user_record, org_config_obj, err := users_manager.GetUserFromContext(ctx)
	if err != nil {
		return nil, Status(self.verbose, err)
	}
	principal := user_record.Name

	permissions := acls.SERVER_ARTIFACT_WRITER
	perm, err := services.CheckAccess(org_config_obj, principal, permissions)
	if !perm || err != nil {
		return nil, PermissionDenied(err,
			"User is not allowed to upload artifact packs.")
	}

	prefix := in.Prefix
	var filter_re *regexp.Regexp
	if in.Filter != "" {
		filter_re, err = regexp.Compile("(?i)" + in.Filter)
		if err != nil {
			return nil, Status(self.verbose, err)
		}
	}

	zip_reader, closer, err := getZipReader(ctx, org_config_obj, in)
	if err != nil {
		return nil, Status(self.verbose, err)
	}
	defer closer()

	result := &api_proto.LoadArtifactPackResponse{
		VfsPath: in.VfsPath,
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

			// Update the definition to include the prefix on the
			// artifact name.
			artifact_definition := ensureArtifactPrefix(
				string(data), in.Prefix)

			request := &api_proto.SetArtifactRequest{
				Op:       api_proto.SetArtifactRequest_CHECK,
				Artifact: artifact_definition,
			}

			definition, err := setArtifactFile(ctx,
				org_config_obj, principal, request, prefix)
			if err != nil {
				continue
			}

			if filter_re != nil && !filter_re.MatchString(definition.Name) {
				continue
			}

			if !in.ReallyDoIt {
				result.SuccessfulArtifacts = append(result.SuccessfulArtifacts,
					definition.Name)
				continue
			}

			request.Op = api_proto.SetArtifactRequest_SET

			// Set the artifact for real.
			definition, err = setArtifactFile(ctx,
				org_config_obj, principal, request, prefix)
			if err == nil {
				services.LogAudit(ctx,
					org_config_obj, principal, "LoadArtifactPack",
					ordereddict.NewDict().
						Set("artifact", definition.Name).
						Set("details", request.Artifact))

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

func getZipReader(
	ctx context.Context,
	config_obj *config_proto.Config,
	in *api_proto.LoadArtifactPackRequest) (*zip.Reader, func() error, error) {

	// Create a temp file and store the data in it.
	if len(in.Data) > 0 {
		// Check the file is a valid zip file first, before we cache
		// it locally.
		buffer := bytes.NewReader(in.Data)
		zipfd, err := zip.NewReader(buffer, int64(len(in.Data)))
		if err != nil {
			return nil, nil, err
		}

		path_manager := paths.NewTempPathManager("")
		file_store_factory := file_store.GetFileStore(config_obj)
		fd, err := file_store_factory.WriteFile(path_manager.Path())
		if err != nil {
			return nil, nil, err
		}
		defer fd.Close()

		_, err = utils.Copy(ctx, fd, bytes.NewReader(in.Data))
		if err != nil {
			return nil, nil, err
		}
		in.VfsPath = path_manager.Path().Components()
		return zipfd, func() error { return nil }, nil
	}

	// Otherwise open the filestore path
	if len(in.VfsPath) < 2 {
		return nil, nil, errors.New("vfs_path should be specified")
	}

	if in.VfsPath[0] != paths.TEMP_ROOT.Components()[0] {
		return nil, nil, errors.New("vfs_path should be a temp path")
	}

	pathspec := path_specs.NewUnsafeFilestorePath(in.VfsPath...).
		SetType(api.PATH_TYPE_FILESTORE_ANY)

	file_store_factory := file_store.GetFileStore(config_obj)
	fd, err := file_store_factory.ReadFile(pathspec)
	if err != nil {
		return nil, nil, err
	}

	stat, err := fd.Stat()
	if err != nil {
		return nil, nil, err
	}

	zip_reader, err := zip.NewReader(
		utils.MakeReaderAtter(fd), stat.Size())
	return zip_reader, fd.Close, err
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
