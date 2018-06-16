//
package binary

import (
	"encoding/json"
	"fmt"
	"io"
	//utils "www.velocidex.com/golang/velociraptor/testing"
)

type _Fields map[string][]*json.RawMessage
type _JsonTypes map[string][]*json.RawMessage

type Profile struct {
	types map[string]Parser
}

func NewProfile() *Profile {
	result := Profile{
		types: make(map[string]Parser),
	}

	return &result
}

func (self *Profile) getParser(name string) (Parser, bool) {
	parser, pres := self.types[name]
	if pres {
		return parser, true
	}

	// If we can not find the parser we assume it is not defined
	// yet and so we provide a late binding parser.
	return nil, false
}

func (self *Profile) StructSize(name string, offset int64, reader io.ReaderAt) int64 {
	parser, pres := self.types[name]
	if pres {
		return parser.Size(offset, reader)
	}

	return 0
}

func (self *Profile) ParseStructDefinitions(definitions string) error {
	var types _JsonTypes

	err := json.Unmarshal([]byte(definitions), &types)
	if err != nil {
		return err
	}

	for type_name, definition_list := range types {
		var size int64
		err := json.Unmarshal(*definition_list[0], &size)
		if err != nil {
			return err
		}

		struct_parser := NewStructParser(type_name, size)
		self.types[type_name] = struct_parser

		var fields _Fields
		err = json.Unmarshal(*definition_list[1], &fields)
		if err != nil {
			return err
		}

		for field_name, field_def := range fields {
			var offset int64
			err := json.Unmarshal(*field_def[0], &offset)
			if err != nil {
				return err
			}

			var params []json.RawMessage
			err = json.Unmarshal(*field_def[1], &params)
			if err != nil {
				return err
			}

			var parser_name string
			err = json.Unmarshal(params[0], &parser_name)
			if err != nil {
				return err
			}

			parser := &ParseAtOffset{
				offset:    offset,
				name:      field_name,
				profile:   self,
				type_name: parser_name}

			parser.SetName(field_name)
			if len(params) == 2 {
				err := parser.ParseArgs(&params[1])
				if err != nil {
					return err
				}
			}

			struct_parser.AddParser(
				field_name,
				parser)
		}
	}

	return nil
}

func (self *Profile) Create(type_name string, offset int64, reader io.ReaderAt) Object {
	parser, pres := self.types[type_name]
	if !pres {
		return &ErrorObject{
			fmt.Sprintf("Type name %s is not known.", type_name)}
	}

	return &BaseObject{
		offset:    offset,
		reader:    reader,
		type_name: type_name,
		name:      type_name,
		parser:    parser}
}
