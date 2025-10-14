package json

import (
	"github.com/Velocidex/ordereddict"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protoreflect"
)

// Convert a protobuf to an ordered dict.  Ordered Dicts have more
// predictable json serializations and are therefore more desirable
// for VQL export:
//
// 1) The serialized fields are exactly the same casing as the accessed fields.
// 2) Enums are encoded in the same way as they are accessed.
//
// This function is usually used to convert a protobuf in a VQL
// plugin/function before emitting the message into the VQL subsystem.
func ConvertProtoToOrderedDict(m proto.Message) *ordereddict.Dict {
	return descriptorToDict(m.ProtoReflect())
}

func descriptorToDict(message protoreflect.Message) *ordereddict.Dict {
	descriptor := message.Descriptor()
	result := ordereddict.NewDict()

	fields := descriptor.Fields()
	for i := 0; i < fields.Len(); i++ {
		field := fields.Get(i)

		value := message.Get(field)

		if field.Cardinality() == protoreflect.Repeated {
			if field.IsMap() {
				setMapValue(result, value, field)
			} else {
				setRepeatedValue(result, value, field)
			}
		} else {
			setOneValue(result, value, field)
		}
	}

	return result
}

func setMapValue(result *ordereddict.Dict, value protoreflect.Value,
	field protoreflect.FieldDescriptor) {
	field_name := string(field.Name())

	m := ordereddict.NewDict()
	value.Map().Range(func(key protoreflect.MapKey, value protoreflect.Value) bool {
		m.Set(key.String(), value.Interface())
		return true
	})

	result.Set(field_name, m)
}

func setRepeatedValue(result *ordereddict.Dict, value protoreflect.Value,
	field protoreflect.FieldDescriptor) {

	field_name := string(field.Name())

	switch field.Kind() {
	case protoreflect.MessageKind:
		value_list := value.List()
		list := make([]*ordereddict.Dict, 0, value_list.Len())
		for i := 0; i < value_list.Len(); i++ {
			new_value := descriptorToDict(value_list.Get(i).Message())
			list = append(list, new_value)
		}
		result.Set(field_name, list)

	case protoreflect.EnumKind:
		value_list := value.List()
		list := make([]string, 0, value_list.Len())
		enum_desc := field.Enum().Values()

		for i := 0; i < value_list.Len(); i++ {
			value := value_list.Get(i)
			value_descriptor := enum_desc.ByNumber(value.Enum())
			list = append(list, string(value_descriptor.Name()))
		}
		result.Set(field_name, list)

	default:
		value_list := value.List()
		list := make([]interface{}, 0, value_list.Len())
		for i := 0; i < value_list.Len(); i++ {
			list = append(list, value_list.Get(i).Interface())
		}
		result.Set(field_name, list)
	}

}

func setOneValue(result *ordereddict.Dict, value protoreflect.Value,
	field protoreflect.FieldDescriptor) {
	field_name := string(field.Name())
	switch field.Kind() {

	// Store the enums as strings so we can match them properly in
	// VQL.
	case protoreflect.EnumKind:
		value_descriptor := field.Enum().Values().ByNumber(value.Enum())
		if value_descriptor != nil {
			result.Set(field_name, string(value_descriptor.Name()))
		}

	case protoreflect.MessageKind:
		result.Set(field_name, descriptorToDict(value.Message()))

	default:
		// Skip empty values
		v := value.Interface()
		result.Set(field_name, v)
	}
}
