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

	"github.com/golang/protobuf/ptypes/empty"
	context "golang.org/x/net/context"
	api_proto "www.velocidex.com/golang/velociraptor/api/proto"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	"www.velocidex.com/golang/velociraptor/services"
	vql_subsystem "www.velocidex.com/golang/velociraptor/vql"
	"www.velocidex.com/golang/vfilter"
)

var (
	doc_regex = regexp.MustCompile("doc=(.+)")
)

func (self *ApiServer) GetKeywordCompletions(
	ctx context.Context,
	in *empty.Empty) (*api_proto.KeywordCompletions, error) {

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

	scope := vql_subsystem.MakeScope()
	defer scope.Close()

	type_map := vfilter.NewTypeMap()
	info := scope.Describe(type_map)

	for _, item := range info.Functions {
		result.Items = append(result.Items, &api_proto.Completion{
			Name:        item.Name,
			Description: item.Doc,
			Type:        "Function",
			Args:        getArgDescriptors(item.ArgType, type_map, scope),
		})
	}

	for _, item := range info.Plugins {
		result.Items = append(result.Items, &api_proto.Completion{
			Name:        item.Name,
			Description: item.Doc,
			Type:        "Plugin",
			Args:        getArgDescriptors(item.ArgType, type_map, scope),
		})
	}

	repository, err := services.GetRepositoryManager().GetGlobalRepository(self.config)
	if err != nil {
		return nil, err
	}
	for _, name := range repository.List() {
		result.Items = append(result.Items, &api_proto.Completion{
			Name: "Artifact." + name,
			Type: "Artifact",
			Args: getArtifactParamDescriptors(
				self.config, name, type_map, repository),
		})
	}

	return result, nil
}

func getArgDescriptors(arg_type string, type_map *vfilter.TypeMap,
	scope *vfilter.Scope) []*api_proto.ArgDescriptor {
	args := []*api_proto.ArgDescriptor{}
	arg_desc, pres := type_map.Get(scope, arg_type)
	if pres && arg_desc != nil && arg_desc.Fields != nil {
		for _, k := range arg_desc.Fields.Keys() {
			v_any, _ := arg_desc.Fields.Get(k)
			v, ok := v_any.(*vfilter.TypeReference)
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

func getArtifactParamDescriptors(
	config_obj *config_proto.Config,
	name string, type_map *vfilter.TypeMap,
	repository services.Repository) []*api_proto.ArgDescriptor {
	args := []*api_proto.ArgDescriptor{}
	artifact, pres := repository.Get(config_obj, name)
	if !pres {
		return args
	}

	for _, parameter := range artifact.Parameters {
		args = append(args, &api_proto.ArgDescriptor{
			Name:        parameter.Name,
			Description: parameter.Description,
			Type:        "Artifact Parameter",
		})
	}

	return args
}
