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
	"encoding/json"
	"errors"
	"path"
	"regexp"
	"strings"

	actions_proto "www.velocidex.com/golang/velociraptor/actions/proto"
	api_proto "www.velocidex.com/golang/velociraptor/api/proto"
	"www.velocidex.com/golang/velociraptor/artifacts"
	artifacts_proto "www.velocidex.com/golang/velociraptor/artifacts/proto"
	"www.velocidex.com/golang/velociraptor/constants"
	file_store "www.velocidex.com/golang/velociraptor/file_store"
	"www.velocidex.com/golang/velociraptor/utils"
)

const (
	default_artifact = `name: Artifact.Name.In.Category
description: |
   This is the human readable description of the artifact.

parameters:
   - name: FirstParameter
     default: Default Value of first parameter

sources:
  - precondition:
      SELECT OS From info() where OS = 'windows'

    queries:
    - |
      SELECT * FROM scope()
`
)

func getArtifactFile(
	config_obj *api_proto.Config,
	vfs_path string) (string, error) {

	vfs_path = path.Clean(vfs_path)
	if vfs_path == "" || !strings.HasSuffix(vfs_path, ".yaml") {
		return default_artifact, nil
	}

	fd, err := getFileForVFSPath(config_obj, "", vfs_path)
	if err == nil {
		defer fd.Close()

		artifact := make([]byte, 1024*10)
		n, err := fd.Read(artifact)
		if err == nil {
			return string(artifact[:n]), nil
		}
		return "", err
	}

	return default_artifact, nil
}

func setArtifactFile(config_obj *api_proto.Config,
	vfs_path string, artifact string) error {

	vfs_path = path.Clean(vfs_path)
	if vfs_path == "" || !strings.HasSuffix(vfs_path, ".yaml") {
		return errors.New("artifact filename must end with .yaml")
	}

	if !strings.HasPrefix(vfs_path, constants.ARTIFACT_DEFINITION) {
		return errors.New("artifacts may only be stored in " +
			constants.ARTIFACT_DEFINITION)
	}

	// First ensure that the artifact is correct.
	tmp_repository := artifacts.NewRepository()
	artifact_obj, err := tmp_repository.LoadYaml(artifact)
	if err != nil {
		return err
	}
	artifact_obj.Path = strings.TrimPrefix(
		vfs_path, constants.ARTIFACT_DEFINITION)

	// Now write it into the filestore.
	file_store_factory := file_store.GetFileStore(config_obj)
	fd, err := file_store_factory.WriteFile(vfs_path)
	if err != nil {
		return err
	}
	defer fd.Close()

	// We want to completely replace the content of the file.
	fd.Truncate(0)

	_, err = fd.Write([]byte(artifact))
	if err != nil {
		return err
	}

	// Load the new artifact into the global repo so it is
	// immediately available.
	global_repository, err := artifacts.GetGlobalRepository(config_obj)
	if err != nil {
		return err
	}
	_, err = global_repository.LoadYaml(artifact)
	return err
}

func renderBuiltinArtifacts(
	config_obj *api_proto.Config,
	vfs_path string) (*actions_proto.VQLResponse, error) {
	repository, err := artifacts.GetGlobalRepository(config_obj)
	if err != nil {
		return nil, err
	}
	directories := []string{}
	artifacts := []*artifacts_proto.Artifact{}
	artifact_path := path.Join("/", strings.TrimPrefix(
		vfs_path, constants.BUILTIN_ARTIFACT_DEFINITION))

	for _, artifact_obj := range repository.Data {
		if !strings.HasPrefix(artifact_obj.Path, artifact_path) {
			continue
		}

		components := []string{}
		for _, item := range strings.Split(
			strings.TrimPrefix(artifact_obj.Path, artifact_path),
			"/") {
			if item != "" {
				components = append(components, item)
			}
		}

		if len(components) > 1 && !utils.InString(&directories, components[0]) {
			directories = append(directories, components[0])
		} else if len(components) == 1 {
			artifacts = append(artifacts, artifact_obj)
		}
	}

	var rows []*FileInfoRow
	for _, dirname := range directories {
		rows = append(rows, &FileInfoRow{
			Name: dirname,
			Mode: "dr-xr-xr-x",
		})
	}

	for _, artifact_obj := range artifacts {
		rows = append(rows, &FileInfoRow{
			Name: path.Base(artifact_obj.Path),
			Mode: "-r--r--r--",
			Size: int64(len(artifact_obj.Raw)),
			Download: &DownloadInfo{
				VfsPath: path.Join(
					vfs_path, path.Base(artifact_obj.Path)),
				Size: int64(len(artifact_obj.Raw)),
			},
		})
	}

	encoded_rows, err := json.MarshalIndent(rows, "", " ")
	if err != nil {
		return nil, err
	}

	return &actions_proto.VQLResponse{
		Columns: []string{
			"Download", "Name", "Size", "Mode", "Timestamp",
		},
		Response: string(encoded_rows),
		Types: []*actions_proto.VQLTypeMap{
			&actions_proto.VQLTypeMap{
				Column: "Download",
				Type:   "Download",
			},
		},
	}, nil
}

func searchArtifact(
	config_obj *api_proto.Config,
	terms []string,
	artifact_type string,
	number_of_results uint64) (
	*artifacts_proto.ArtifactDescriptors, error) {

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
