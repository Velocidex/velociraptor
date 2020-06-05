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
	"reflect"
	"regexp"
	"strings"

	"github.com/golang/protobuf/descriptor"
	"github.com/golang/protobuf/proto"
	descriptor_proto "github.com/golang/protobuf/protoc-gen-go/descriptor"
	"github.com/golang/protobuf/ptypes/empty"
	context "golang.org/x/net/context"
	api_proto "www.velocidex.com/golang/velociraptor/api/proto"
	"www.velocidex.com/golang/velociraptor/artifacts"
	artifacts_proto "www.velocidex.com/golang/velociraptor/artifacts/proto"
	semantic_proto "www.velocidex.com/golang/velociraptor/proto"
	vql_subsystem "www.velocidex.com/golang/velociraptor/vql"
	"www.velocidex.com/golang/vfilter"
)

var (
	// List of all protobufs exported through the APIs.
	EXPORTED_PROTO = []string{
		"StartFlowRequest",
		"StartFlowResponse",
		"SearchClientsRequest",
		"SearchClientsResponse",
		"GetClientRequest",
		"ApiClient",
		"HuntInfo",
		"GrrMessage",
		"VQLCollectorArgs",
		"VQLResponse",
		"Types",
		"Hunt",
		"FlowRunnerArgs",
		"FlowContext",
		"VFSListRequest",
		"FileFinderArgs",
		"VFSDownloadFileRequest",
		"ArtifactCollectorArgs",
		"ArtifactParameter",
		"Artifacts",
	}

	doc_regex = regexp.MustCompile("doc=(.+)")
)

func describeTypes() *artifacts_proto.Types {
	seen := make(map[string]bool)
	result := &artifacts_proto.Types{
		Items: []*artifacts_proto.TypeDescriptor{
			{Name: "ByteSize", Kind: "primitive", Default: "0"},
			{Name: "GlobExpression", Kind: "primitive", Default: "\"\""},
			{Name: "RegularExpression", Kind: "primitive", Default: "\"\""},
			{Name: "LiteralExpression", Kind: "primitive", Default: "\"\""},
			{Name: "ClientURN", Kind: "primitive", Default: "\"\""},
			{Name: "RDFURN", Kind: "primitive", Default: "\"\""},
			{Name: "bool", Kind: "primitive", Default: "false"},
			{Name: "string", Kind: "primitive", Default: "\"\""},
			{Name: "integer", Kind: "primitive", Default: "0"},
			{Name: "ApiClientId", Kind: "primitive", Default: "\"\""},
			{Name: "RDFDatetime", Kind: "primitive", Default: "0"},
			{Name: "RDFDatetimeSeconds", Kind: "primitive", Default: "0"},
		},
	}
	for _, proto_name := range EXPORTED_PROTO {
		add_type(proto_name, result, seen)
	}

	return result
}

func add_type(type_name string, result *artifacts_proto.Types, seen map[string]bool) {
	message_type := proto.MessageType("proto." + type_name)
	if message_type == nil {
		return
	}

	// Prevent loops.
	if _, pres := seen[type_name]; pres {
		return
	}
	seen[type_name] = true

	new_message := reflect.New(message_type.Elem()).Interface().(descriptor.Message)
	_, md := descriptor.ForMessage(new_message)
	type_desc := &artifacts_proto.TypeDescriptor{
		Name: type_name,
		Kind: "struct",
	}

	opts := md.GetOptions()
	ext, err := proto.GetExtension(opts, semantic_proto.E_Semantic)
	if err == nil {
		sem_ext, ok := ext.(*semantic_proto.SemanticMessageDescriptor)
		if ok {
			type_desc.Doc = sem_ext.Description
			type_desc.FriendlyName = sem_ext.FriendlyName
		}
	}

	if md.OneofDecl != nil {
		type_desc.Oneof = true
	}

	result.Items = append(result.Items, type_desc)
	seen[type_name] = true

	for _, field := range md.Field {
		field_descriptor := &artifacts_proto.FieldDescriptor{
			Type:    getFieldType(field),
			Default: getFieldDefaults(field),
		}

		if field.Label != nil &&
			*field.Label ==
				descriptor_proto.FieldDescriptorProto_LABEL_REPEATED {
			field_descriptor.Repeated = true
			field_descriptor.Default = "[]"
		}

		opts := field.GetOptions()
		ext, err := proto.GetExtension(opts, semantic_proto.E_SemType)
		if err == nil {
			sem_ext, ok := ext.(*semantic_proto.SemanticDescriptor)
			if ok {
				if sem_ext.Type != "" {
					field_descriptor.Type = sem_ext.Type
				}

				if sem_ext.Default != "" {
					field_descriptor.Default = sem_ext.Default
				}

				field_descriptor.Doc = sem_ext.Description
				field_descriptor.FriendlyName = sem_ext.FriendlyName
				for _, label := range sem_ext.Label {
					if label == semantic_proto.SemanticDescriptor_HIDDEN {
						field_descriptor.Labels = append(
							field_descriptor.Labels, "HIDDEN")
					}
				}
			}
		}

		if field.Name != nil {
			field_descriptor.Name = *field.Name
		}
		if field.Type != nil &&
			*field.Type == descriptor_proto.FieldDescriptorProto_TYPE_ENUM {
			describe_enum(field, result, seen, field_descriptor)
		}

		if field.TypeName != nil {
			type_name := strings.TrimPrefix(*field.TypeName, ".proto.")
			add_type(type_name, result, seen)
		}

		type_desc.Fields = append(type_desc.Fields, field_descriptor)
	}
}

func describe_enum(
	field *descriptor_proto.FieldDescriptorProto,
	result *artifacts_proto.Types,
	seen map[string]bool,
	descriptor *artifacts_proto.FieldDescriptor) {
	if field.TypeName == nil {
		return
	}
	type_name := strings.TrimPrefix(*field.TypeName, ".proto.")
	type_name = strings.Replace(type_name, ".", "_", -1)
	full_type_name := "proto." + type_name
	message_type := proto.EnumValueMap(full_type_name)
	if message_type != nil {
		descriptor.Type = type_name

		type_desc := &artifacts_proto.TypeDescriptor{
			Name: type_name,
			Kind: "enum",
		}

		for name, value := range message_type {
			type_desc.AllowedValues = append(
				type_desc.AllowedValues,
				&artifacts_proto.EnumValue{Name: name, Value: value})
		}

		type_desc.Name = type_name
		type_desc.Default = "\"" + type_desc.AllowedValues[0].Name + "\""

		// Prevent loops.
		if _, pres := seen[type_name]; pres {
			return
		}
		descriptor.Default = type_desc.Default

		result.Items = append(result.Items, type_desc)
	}
}

func getFieldType(desc *descriptor_proto.FieldDescriptorProto) string {
	switch *desc.Type {
	case descriptor_proto.FieldDescriptorProto_TYPE_BOOL:
		return "bool"

	case descriptor_proto.FieldDescriptorProto_TYPE_DOUBLE:
		return "double"

	case descriptor_proto.FieldDescriptorProto_TYPE_INT64,
		descriptor_proto.FieldDescriptorProto_TYPE_UINT64,
		descriptor_proto.FieldDescriptorProto_TYPE_FIXED64,
		descriptor_proto.FieldDescriptorProto_TYPE_INT32,
		descriptor_proto.FieldDescriptorProto_TYPE_UINT32:
		return "integer"

	case descriptor_proto.FieldDescriptorProto_TYPE_MESSAGE:
		if desc.Type != nil {
			return strings.TrimPrefix(*desc.TypeName, ".proto.")
		}
		return "string"

	default:
		return "string"
	}
}

func getFieldDefaults(desc *descriptor_proto.FieldDescriptorProto) string {
	switch *desc.Type {
	case descriptor_proto.FieldDescriptorProto_TYPE_BOOL:
		return "false"

	case descriptor_proto.FieldDescriptorProto_TYPE_DOUBLE:
		return "0"

	case descriptor_proto.FieldDescriptorProto_TYPE_INT64,
		descriptor_proto.FieldDescriptorProto_TYPE_UINT64,
		descriptor_proto.FieldDescriptorProto_TYPE_FIXED64,
		descriptor_proto.FieldDescriptorProto_TYPE_INT32,
		descriptor_proto.FieldDescriptorProto_TYPE_UINT32:
		return "0"

	case descriptor_proto.FieldDescriptorProto_TYPE_MESSAGE:
		return "{}"

	default:
		return "\"\""
	}
}

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

	repository, err := artifacts.GetGlobalRepository(self.config)
	if err != nil {
		return nil, err
	}
	for _, name := range repository.List() {
		result.Items = append(result.Items, &api_proto.Completion{
			Name: "Artifact." + name,
			Type: "Artifact",
			Args: getArtifactParamDescriptors(name, type_map, repository),
		})
	}

	return result, nil
}

func getArgDescriptors(arg_type string, type_map *vfilter.TypeMap,
	scope *vfilter.Scope) []*api_proto.ArgDescriptor {
	args := []*api_proto.ArgDescriptor{}
	arg_desc, pres := type_map.Get(scope, arg_type)
	if pres {
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

func getArtifactParamDescriptors(name string, type_map *vfilter.TypeMap,
	repository *artifacts.Repository) []*api_proto.ArgDescriptor {
	args := []*api_proto.ArgDescriptor{}
	artifact, pres := repository.Get(name)
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
