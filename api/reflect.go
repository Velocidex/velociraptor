package api

import (
	"github.com/golang/protobuf/descriptor"
	"github.com/golang/protobuf/proto"
	"reflect"
	"strings"

	descriptor_proto "github.com/golang/protobuf/protoc-gen-go/descriptor"
	api_proto "www.velocidex.com/golang/velociraptor/api/proto"
	semantic_proto "www.velocidex.com/golang/velociraptor/proto"
	utils "www.velocidex.com/golang/velociraptor/testing"
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
		"GrrMessage",
		"VQLCollectorArgs",
		"VQLResponse",
		"Types",
		"FlowRunnerArgs",
		"FlowContext",
		"VInterrogateArgs",
		"VFSListRequest",
		"FileFinderArgs",
	}
)

func describeTypes() *api_proto.Types {
	seen := make(map[string]bool)
	result := &api_proto.Types{
		Items: []*api_proto.TypeDescriptor{
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
		},
	}
	for _, proto_name := range EXPORTED_PROTO {
		add_type(proto_name, result, seen)
	}

	return result
}

func add_type(type_name string, result *api_proto.Types, seen map[string]bool) {
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
	if type_name == "FileFinderAction" {
		utils.Debug(md)
	}
	type_desc := &api_proto.TypeDescriptor{
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
		field_descriptor := &api_proto.FieldDescriptor{
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
			describe_enum(field, field_descriptor)
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
	descriptor *api_proto.FieldDescriptor) {
	if field.TypeName == nil {
		return
	}
	type_name := strings.TrimPrefix(*field.TypeName, ".proto.")
	type_name = strings.Replace(type_name, ".", "_", -1)
	type_name = "proto." + type_name
	message_type := proto.EnumValueMap(type_name)
	if message_type != nil {
		for name, value := range message_type {
			descriptor.AllowedValues = append(
				descriptor.AllowedValues,
				&api_proto.EnumValue{Name: name, Value: value})
		}

		descriptor.Type = "EnumNamedValue"
		descriptor.Default = "\"" + descriptor.AllowedValues[0].Name + "\""
	}
	utils.Debug(message_type)

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
