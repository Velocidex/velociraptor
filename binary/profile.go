//
package binary

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
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

			// When we parse the JSON definition we place
			// a delayed reference ParseAtOffset object as
			// an intermediate. The struct will
			// dereference its fields through the psuedo
			// parser which will fetch the real parser
			// dynamically.
			parser := &ParseAtOffset{
				offset:    offset,
				name:      field_name,
				profile:   self,
				type_name: parser_name,
			}

			parser.SetName(field_name)
			if len(params) == 2 {
				parser.ParseArgs(&params[1])
			}

			struct_parser.AddParser(field_name, parser)
		}
	}

	return nil
}

// Create a new object of the specified type. For example:
// type_name = "Array"
// options = { "Target": "int"}
func (self *Profile) Create(type_name string, offset int64,
	reader io.ReaderAt, options map[string]interface{}) (Object, error) {
	var parser Parser

	profile_parser, pres := self.types[type_name]
	if !pres {
		return nil, errors.New(
			fmt.Sprintf("Type name %s is not known.", type_name))
	}

	// We need a new copy of the parser since the params might be
	// unique.
	parser = profile_parser.Copy()

	// Convert the options map into json.RawMessage so we can make
	// the parser parse it.
	message, err := json.Marshal(options)
	if err != nil {
		return nil, err
	}
	raw_message := json.RawMessage(message)
	parser.ParseArgs(&raw_message)

	return &BaseObject{
		offset:    offset,
		reader:    reader,
		type_name: type_name,
		name:      type_name,
		parser:    parser,
	}, nil
}
