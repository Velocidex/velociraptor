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
	"regexp"
	"strings"

	"github.com/Velocidex/yaml/v2"
	context "golang.org/x/net/context"
	"google.golang.org/protobuf/types/known/emptypb"
	api_proto "www.velocidex.com/golang/velociraptor/api/proto"
	"www.velocidex.com/golang/velociraptor/artifacts/assets"
	artifacts_proto "www.velocidex.com/golang/velociraptor/artifacts/proto"
	"www.velocidex.com/golang/velociraptor/services"
	vql_subsystem "www.velocidex.com/golang/velociraptor/vql"
	"www.velocidex.com/golang/vfilter"
	"www.velocidex.com/golang/vfilter/types"
)

var (
	doc_regex = regexp.MustCompile("doc=(.+)")
)

// Loads the api description from the embedded asset
func LoadApiDescription() ([]*api_proto.Completion, error) {
	assets.Init()

	data, err := assets.ReadFile("docs/references/vql.yaml")
	if err != nil {
		return nil, err
	}

	result := []*api_proto.Completion{}
	err = yaml.Unmarshal(data, &result)
	return result, err
}

func IntrospectDescription() []*api_proto.Completion {
	result := []*api_proto.Completion{}

	scope := vql_subsystem.MakeScope()
	defer scope.Close()

	type_map := types.NewTypeMap()
	info := scope.Describe(type_map)

	for _, item := range info.Functions {
		result = append(result, &api_proto.Completion{
			Name:        item.Name,
			Description: item.Doc,
			Type:        "Function",
			Args:        getArgDescriptors(item.ArgType, type_map, scope),
		})
	}

	for _, item := range info.Plugins {
		result = append(result, &api_proto.Completion{
			Name:        item.Name,
			Description: item.Doc,
			Type:        "Plugin",
			Args:        getArgDescriptors(item.ArgType, type_map, scope),
		})
	}

	return result
}

func (self *ApiServer) GetKeywordCompletions(
	ctx context.Context,
	in *emptypb.Empty) (*api_proto.KeywordCompletions, error) {

	users := services.GetUserManager()
	_, org_config_obj, err := users.GetUserFromContext(ctx)
	if err != nil {
		return nil, err
	}

	result := &api_proto.KeywordCompletions{
		Items: []*api_proto.Completion{
			{Name: "SELECT", Type: "Keyword"},
			{Name: "FROM", Type: "Keyword"},
			{Name: "LET", Type: "Keyword"},
			{Name: "WHERE", Type: "Keyword"},
			{Name: "LIMIT", Type: "Keyword"},
			{Name: "GROUP BY", Type: "Keyword"},
			{Name: "ORDER BY", Type: "Keyword"},
		},
	}

	descriptions, err := LoadApiDescription()
	if err != nil {
		descriptions = IntrospectDescription()
	}
	result.Items = append(result.Items, descriptions...)

	manager, err := services.GetRepositoryManager()
	if err != nil {
		return nil, err
	}
	repository, err := manager.GetGlobalRepository(org_config_obj)
	if err != nil {
		return nil, err
	}
	names, err := repository.List(ctx, org_config_obj)
	if err != nil {
		return nil, err
	}

	for _, name := range names {
		artifact, pres := repository.Get(org_config_obj, name)
		if !pres {
			continue
		}
		result.Items = append(result.Items, &api_proto.Completion{
			Name: "Artifact." + name,
			Type: "Artifact",
			Args: getArtifactParamDescriptors(artifact),
		})
	}

	return result, nil
}

func getArgDescriptors(
	arg_type string,
	type_map *vfilter.TypeMap,
	scope vfilter.Scope) []*api_proto.ArgDescriptor {
	args := []*api_proto.ArgDescriptor{}
	arg_desc, pres := type_map.Get(scope, arg_type)
	if pres && arg_desc != nil && arg_desc.Fields != nil {
		for _, k := range arg_desc.Fields.Keys() {
			v_any, _ := arg_desc.Fields.Get(k)
			v, ok := v_any.(*types.TypeReference)
			if !ok {
				continue
			}

			target := v.Target
			if v.Repeated {
				target = " list of " + target
			}

			required := ""
			if strings.Contains(v.Tag, "required") {
				required = "(required)"
			}
			doc := ""
			matches := doc_regex.FindStringSubmatch(v.Tag)
			if matches != nil {
				doc = matches[1]
			}
			args = append(args, &api_proto.ArgDescriptor{
				Name:        k,
				Description: doc + required,
				Type:        target,
			})
		}
	}
	return args
}

func getArtifactParamDescriptors(artifact *artifacts_proto.Artifact) []*api_proto.ArgDescriptor {
	args := []*api_proto.ArgDescriptor{}

	for _, parameter := range artifact.Parameters {
		args = append(args, &api_proto.ArgDescriptor{
			Name:        parameter.Name,
			Description: parameter.Description,
			Type:        "Artifact Parameter",
		})
	}

	return args
}
