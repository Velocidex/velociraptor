/*
Velociraptor - Dig Deeper
Copyright (C) 2019-2025 Rapid7 Inc.

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
	"context"
	"regexp"
	"strings"

	"github.com/Velocidex/ordereddict"
	errors "github.com/go-errors/errors"
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
	"www.velocidex.com/golang/velociraptor/logging"
	"www.velocidex.com/golang/velociraptor/paths"
	"www.velocidex.com/golang/velociraptor/services"
	"www.velocidex.com/golang/velociraptor/services/launcher"
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

# Can be CLIENT, CLIENT_EVENT, SERVER, SERVER_EVENT or NOTEBOOK
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
		result, err := manager.SetArtifactFile(ctx,
			config_obj, principal, in.Artifact, required_prefix)
		if err != nil {
			return nil, Status(config_obj.Verbose, err)
		}

		if len(in.Tags) > 0 {
			err = manager.SetArtifactMetadata(ctx, config_obj, principal,
				result.Name, &artifacts_proto.ArtifactMetadata{
					Tags: in.Tags,
				})
			if err != nil {
				return nil, Status(config_obj.Verbose, err)
			}
		}

		return result, nil

	}

	return nil, InvalidStatus("Unknown op")
}

func checkArtifact(
	ctx context.Context,
	config_obj *config_proto.Config,
	artifact string) (*launcher.AnalysisState, error) {

	state := launcher.NewAnalysisState(artifact)
	manager, err := services.GetRepositoryManager(config_obj)
	if err != nil {
		return nil, err
	}

	repository, err := manager.GetGlobalRepository(config_obj)
	if err != nil {
		return nil, err
	}

	// Load it into a local repository for checking - this will
	// not commit it to the global repository yet
	local_repository := manager.NewRepository()
	local_repository.SetParent(repository, config_obj)

	artifact_obj, err := local_repository.LoadYaml(artifact,
		services.ArtifactOptions{
			ValidateArtifact: true,
		})

	if err != nil {
		return &launcher.AnalysisState{
			Errors: []string{err.Error()},
		}, nil
	}

	// Verify the artifact
	launcher.VerifyArtifact(
		ctx, config_obj, repository, artifact_obj, state)

	return state, nil
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

	// Show hidden artifacts
	hidden bool

	// Show empty artifacts (those without sources)
	empty_source bool

	builtin *bool

	// Show basic artifacts
	basic *bool

	tags []string
}

func (self *matchPlan) matchTag(artifact *artifacts_proto.Artifact) bool {
	if len(self.tags) == 0 {
		return true
	}

	if artifact.Metadata == nil || len(artifact.Metadata.Tags) == 0 {
		return false
	}

	for _, i := range self.tags {
		for _, j := range artifact.Metadata.Tags {
			if strings.EqualFold(i, j) {
				return true
			}
		}
	}

	return false
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

func (self *matchPlan) matchMetadata(artifact *artifacts_proto.Artifact) bool {
	if self.basic == nil {
		return true
	}

	if *self.basic && artifact.Metadata != nil &&
		artifact.Metadata.Basic {
		return true
	}
	return false
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

func (self *matchPlan) hideEmptySources() bool {
	// User wants to show empty sources
	if self.empty_source {
		return false
	}

	// Tag searches should show all artifacts - including ones without
	// sources.
	if len(self.tags) > 0 {
		return false
	}

	return true
}

// All conditions must match
func (self *matchPlan) matchArtifact(artifact *artifacts_proto.Artifact) bool {
	if !self.hidden && // Dont show hidden artifacts

		// Artifact is set to hidden
		artifact.Metadata != nil && artifact.Metadata.Hidden {
		return false
	}

	if self.hideEmptySources() && len(artifact.Sources) == 0 {
		return false
	}

	if !self.matchType(artifact) {
		return false
	}

	if !self.matchDescOrName(artifact) {
		return false
	}

	if !self.matchTag(artifact) {
		return false
	}

	if !self.matchPreconditions(artifact) {
		return false
	}

	if !self.matchBuiltin(artifact) {
		return false
	}

	if !self.matchMetadata(artifact) {
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
			case "empty":
				if term == "true" {
					result.empty_source = true
				}
				continue

			case "hidden":
				if term == "true" {
					result.hidden = true
				}
				continue

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

			case "metadata":
				if term == "basic" {
					value := true
					result.basic = &value
				}
				continue

			case "tag":
				result.tags = append(result.tags, strings.ToLower(term))
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
					new_item.IsInherited = artifact.IsInherited
				}

				if fields.Description {
					new_item.Description = artifact.Description
				}
				if fields.Type {
					new_item.Type = artifact.Type
				}

				if fields.Sources {
					for _, s := range artifact.Sources {
						new_item.Sources = append(new_item.Sources,
							&artifacts_proto.ArtifactSource{
								Name:        s.Name,
								Description: s.Description,
							})
					}
				}

				result.Items = append(result.Items, new_item)
			}
		}

		if len(result.Items) >= int(number_of_results) {
			break
		}
	}

	if fields != nil && fields.Tags {
		result.Tags, err = repository.Tags(ctx, config_obj)
		if err != nil {
			return nil, Status(config_obj.Verbose, err)
		}
	}

	return result, nil
}

func (self *ApiServer) LoadArtifactPack(
	ctx context.Context,
	in *api_proto.LoadArtifactPackRequest) (
	res *api_proto.LoadArtifactPackResponse, err error) {

	defer Instrument("LoadArtifactPack")()

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
	defer func() {
		err1 := closer()
		if err != nil {
			err = err1
		}
	}()

	result := &api_proto.LoadArtifactPackResponse{
		VfsPath: in.VfsPath,
	}
	for _, file := range zip_reader.File {
		if strings.HasSuffix(file.Name, ".yaml") ||
			strings.HasSuffix(file.Name, ".yml") {
			fd, err := file.Open()
			if err != nil {
				continue
			}

			data, err := utils.ReadAllWithLimit(fd, constants.MAX_MEMORY)
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
				Tags:     in.Tags,
			}

			definition, err := setArtifactFile(ctx,
				org_config_obj, principal, request, prefix)
			if err != nil {
				if len(result.Errors) < 10 {
					result.Errors = append(result.Errors, &api_proto.LoadArtifactError{
						Filename: file.Name,
						Error:    err.Error(),
					})

				} else if len(result.Errors) == 10 {
					result.Errors = append(result.Errors, &api_proto.LoadArtifactError{
						Filename: file.Name,
						Error:    "Too many errors - suppressing",
					})
				}
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
				err := services.LogAudit(ctx,
					org_config_obj, principal, "LoadArtifactPack",
					ordereddict.NewDict().
						Set("artifact", definition.Name).
						Set("details", request.Artifact))
				if err != nil {
					logger := logging.GetLogger(org_config_obj, &logging.FrontendComponent)
					logger.Error("<red>LoadArtifactPack</> %v %v",
						principal, definition.Name)
				}

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

	if in.VfsPath[0] != paths.TEMP_ROOT.Components()[0] &&
		in.VfsPath[0] != paths.PUBLIC_ROOT.Components()[0] {
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
