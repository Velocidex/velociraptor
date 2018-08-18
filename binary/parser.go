// Implements a binary parsing system.
package binary

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"sort"
	"strings"
)

// Parsers are objects which know how to parse a particular
// type. Parsers are instantiated once and reused many times. They act
// upon an Object which represents a particular instance of a parser
// in a particular offset.

// Here is an example: A struct foo may have 3 members. There is a
// Struct parser instantiated once which knows how to parse struct foo
// (i.e. all its fiels and their offsets). Once instantiated and
// stored in the Profile, the parser may be reused multiple times to
// parse multiple foo structs - each time, it produces an Object.

// The Object struct contains the offset, and the parser that is used
// to parse it.

type Parser interface {
	SetName(name string)
	DebugString(offset int64, reader io.ReaderAt) string
	ShortDebugString(offset int64, reader io.ReaderAt) string
	Size(offset int64, reader io.ReaderAt) int64
	IsValid(offset int64, reader io.ReaderAt) bool
	ParseArgs(args *json.RawMessage) error
	Copy() Parser
}

type Integerer interface {
	AsInteger(offset int64, reader io.ReaderAt) uint64
}

type Stringer interface {
	AsString(offset int64, reader io.ReaderAt) string
}

type Getter interface {
	Get(base Object, field string) Object
	Fields() []string
}

type Iterator interface {
	Value(base Object) Object
	Next(base Object) Object
}

type Object interface {
	Name() string
	AsInteger() uint64
	AsString() string
	Get(field string) Object
	Reader() io.ReaderAt
	Offset() int64
	Size() int64
	DebugString() string
	IsValid() bool
	Value() interface{}
	Fields() []string
	Next() Object
}

type BaseObject struct {
	reader    io.ReaderAt
	offset    int64
	name      string
	type_name string
	parser    Parser
}

func (self *BaseObject) Name() string {
	return self.name
}

func (self *BaseObject) Reader() io.ReaderAt {
	return self.reader
}

func (self *BaseObject) Offset() int64 {
	return self.offset
}

func (self *BaseObject) AsInteger() uint64 {
	switch self.parser.(type) {
	case Integerer:
		return self.parser.(Integerer).AsInteger(self.offset, self.reader)
	default:
		return 0
	}
}

func (self *BaseObject) AsString() string {
	switch t := self.parser.(type) {
	case Stringer:
		return t.AsString(self.offset, self.reader)
	default:
		return ""
	}
}

func (self *BaseObject) Get(field string) Object {
	if strings.Contains(field, ".") {
		components := strings.Split(field, ".")
		var result Object = self
		for _, component := range components {
			result = result.Get(component)
		}

		return result
	}

	switch t := self.parser.(type) {
	case Getter:
		return t.Get(self, field)
	default:
		return NewErrorObject("Parser does not support Get for " + field)
	}
}

func (self *BaseObject) Next() Object {
	switch t := self.parser.(type) {
	case Iterator:
		return t.Next(self)
	default:
		return NewErrorObject("Parser does not support iteration")
	}
}

func (self *BaseObject) DebugString() string {
	return self.parser.DebugString(self.offset, self.reader)
}

func (self *BaseObject) Size() int64 {
	return self.parser.Size(self.offset, self.reader)
}

func (self *BaseObject) IsValid() bool {
	return self.parser.IsValid(self.offset, self.reader)
}

func (self *BaseObject) Value() interface{} {
	switch t := self.parser.(type) {
	case Stringer:
		return self.AsString()
	case Integerer:
		return self.AsInteger()
	case Iterator:
		return t.Value(self)
	default:
		return self
	}
}

func (self *BaseObject) Fields() []string {
	switch t := self.parser.(type) {
	case Getter:
		return t.Fields()
	default:
		return []string{}
	}
}

func (self *BaseObject) MarshalJSON() ([]byte, error) {
	res := make(map[string]interface{})
	for _, field := range self.Fields() {
		res[field] = self.Get(field).Value()
	}
	buf, err := json.Marshal(res)
	return buf, err
}

// When an operation fails we return an error object. The error object
// can continue to be used in all operations and it will just carry
// itself over safely. This means that callers do not need to check
// for errors all the time:

// a.Get("field").Next().Get("field") -> ErrorObject
type ErrorObject struct {
	message string
}

func NewErrorObject(message string) *ErrorObject {
	return &ErrorObject{message}
}

func (self *ErrorObject) Name() string {
	return "Error: " + self.message
}

func (self *ErrorObject) Reader() io.ReaderAt {
	return nil
}

func (self *ErrorObject) Offset() int64 {
	return 0
}

func (self *ErrorObject) Get(field string) Object {
	return self
}

func (self *ErrorObject) Next() Object {
	return self
}

func (self *ErrorObject) AsInteger() uint64 {
	return 0
}

func (self *ErrorObject) AsString() string {
	return ""
}

func (self *ErrorObject) DebugString() string {
	return fmt.Sprintf("Error: %s", self.message)
}

func (self *ErrorObject) Size() int64 {
	return 0
}

func (self *ErrorObject) IsValid() bool {
	return false
}

func (self *ErrorObject) Value() interface{} {
	return errors.New(self.message)
}

func (self *ErrorObject) Fields() []string {
	return []string{}
}

// Baseclass for parsers.
type BaseParser struct {
	Name      string
	size      int64
	type_name string
}

func (self *BaseParser) SetName(name string) {
	self.Name = name
}

func (self *BaseParser) DebugString(offset int64, reader io.ReaderAt) string {
	return fmt.Sprintf("[%s] @ %#0x", self.type_name, offset)
}

func (self *BaseParser) ShortDebugString(offset int64, reader io.ReaderAt) string {
	return fmt.Sprintf("[%s] @ %#0x", self.type_name, offset)
}

func (self *BaseParser) Size(offset int64, reader io.ReaderAt) int64 {
	return self.size
}

func (self *BaseParser) IsValid(offset int64, reader io.ReaderAt) bool {
	buf := make([]byte, self.size)
	_, err := reader.ReadAt(buf, offset)
	if err != nil {
		return false
	}
	return true
}

// If a derived parser takes args. process them here.
func (self *BaseParser) ParseArgs(args *json.RawMessage) error {
	return nil
}

// Parse various sizes of ints.
type IntParser struct {
	*BaseParser
	converter func(buf []byte) uint64
}

func (self IntParser) Copy() Parser {
	return &self
}

func (self *IntParser) DebugString(offset int64, reader io.ReaderAt) string {
	return fmt.Sprintf("[%s] %#0x",
		self.type_name, self.AsInteger(offset, reader))
}

func (self *IntParser) ShortDebugString(offset int64, reader io.ReaderAt) string {
	return fmt.Sprintf("%#0x", self.AsInteger(offset, reader))
}

func (self *IntParser) AsInteger(offset int64, reader io.ReaderAt) uint64 {
	buf := make([]byte, 8)

	n, err := reader.ReadAt(buf, offset)
	if n == 0 || err != nil {
		return 0
	}
	return self.converter(buf)
}

func NewIntParser(type_name string, converter func(buf []byte) uint64) *IntParser {
	return &IntParser{&BaseParser{
		type_name: type_name,
	}, converter}
}

// Parses strings.
type StringParserOptions struct {
	Length int64
}

type StringParser struct {
	*BaseParser
	options *StringParserOptions
}

func NewStringParser(type_name string) *StringParser {
	return &StringParser{&BaseParser{type_name: type_name}, &StringParserOptions{}}
}

func (self StringParser) Copy() Parser {
	return &self
}

func (self *StringParser) AsString(offset int64, reader io.ReaderAt) string {
	read_length := self.options.Length
	if read_length == 0 {
		read_length = 1024
	}
	buf := make([]byte, read_length)

	n, _ := reader.ReadAt(buf, offset)
	result := string(buf[:n])
	idx := strings.IndexByte(result, byte(0))
	if idx >= 0 {
		return result[:idx]
	}

	return result
}

func (self *StringParser) Size(offset int64, reader io.ReaderAt) int64 {
	return int64(len(self.AsString(offset, reader)))
}

func (self *StringParser) DebugString(offset int64, reader io.ReaderAt) string {
	return "[string '" + self.AsString(offset, reader) + "']"
}

func (self *StringParser) ShortDebugString(offset int64, reader io.ReaderAt) string {
	return self.AsString(offset, reader)
}

func (self *StringParser) ParseArgs(args *json.RawMessage) error {
	return json.Unmarshal(*args, &self.options)
}

type StructParser struct {
	*BaseParser
	fields map[string]*ParseAtOffset
}

func (self StructParser) Copy() Parser {
	return &self
}

func (self *StructParser) Get(base Object, field string) Object {
	parser, pres := self.fields[field]
	if pres {
		return parser.Get(base, field)
	}

	return NewErrorObject("Field " + field + " not known.")
}

func (self *StructParser) Fields() []string {
	var result []string
	for k := range self.fields {
		result = append(result, k)
	}

	return result
}

func indent(input string) string {
	var indented []string
	for _, line := range strings.Split(input, "\n") {
		indented = append(indented, "  "+line)
	}

	return strings.Join(indented, "\n")
}

func (self *StructParser) DebugString(offset int64, reader io.ReaderAt) string {
	result := []string{}

	for _, parser := range self.fields {
		result = append(result, indent(parser.DebugString(offset, reader)))
	}
	sort.Strings(result)
	return fmt.Sprintf("[%s] @ %#0x\n", self.type_name, offset) +
		strings.Join(result, "\n")
}

func (self *StructParser) ShortDebugString(offset int64, reader io.ReaderAt) string {
	return fmt.Sprintf("[%s] @ %#0x\n", self.type_name, offset)
}

func (self *StructParser) AddParser(field string, parser *ParseAtOffset) {
	self.fields[field] = parser
}

func NewStructParser(type_name string, size int64) *StructParser {
	result := &StructParser{
		&BaseParser{type_name: type_name, size: size},
		make(map[string]*ParseAtOffset),
	}

	return result
}

type ParseAtOffset struct {
	// Field offset within the struct.
	offset int64
	name   string

	// The name of the parser to use and the params - will be
	// dynamically resolved on first access.
	type_name string
	params    *json.RawMessage

	profile *Profile

	// A local cache of the resolved parser for this field.
	parser Parser
}

func (self *ParseAtOffset) Get(base Object, field string) Object {
	parser, pres := self.getParser(self.type_name)
	if !pres {
		return &ErrorObject{"Type not found"}
	}

	result := &BaseObject{
		name:      field,
		type_name: self.type_name,
		offset:    base.Offset() + self.offset,
		reader:    base.Reader(),
		parser:    parser,
	}
	return result
}

func (self *ParseAtOffset) Fields() []string {
	parser, pres := self.getParser(self.type_name)
	if pres {
		getter, ok := parser.(Getter)
		if ok {
			return getter.Fields()
		}
	}

	return []string{}
}

func (self *ParseAtOffset) DebugString(offset int64, reader io.ReaderAt) string {
	parser, pres := self.getParser(self.type_name)
	if !pres {
		return self.name + ": Type " + self.type_name + " not found"
	}
	return fmt.Sprintf(
		"%#03x  %s  %s", self.offset, self.name,
		parser.DebugString(self.offset+offset, reader))
}

func (self *ParseAtOffset) ShortDebugString(offset int64, reader io.ReaderAt) string {
	parser, pres := self.getParser(self.type_name)
	if !pres {
		return "Type not found"
	}

	return parser.ShortDebugString(self.offset+offset, reader)
}

func (self *ParseAtOffset) SetName(name string) {
	self.name = name
}

func (self *ParseAtOffset) Size(offset int64, reader io.ReaderAt) int64 {
	parser, pres := self.getParser(self.type_name)
	if !pres {
		return 0
	}

	return parser.Size(self.offset+offset, reader)
}

func (self *ParseAtOffset) IsValid(offset int64, reader io.ReaderAt) bool {
	parser, pres := self.getParser(self.type_name)
	if !pres {
		return false
	}

	return parser.IsValid(self.offset+offset, reader)
}

func (self *ParseAtOffset) getParser(name string) (Parser, bool) {
	// Get parser from the cache if possible.
	if self.parser != nil {
		return self.parser, true
	}

	parser, pres := self.profile.getParser(self.type_name)
	if !pres {
		return nil, false
	}

	// Prepare a new parser based on the params.
	self.parser = parser.Copy()
	self.parser.ParseArgs(self.params)

	return self.parser, true
}

func (self *ParseAtOffset) ParseArgs(args *json.RawMessage) {
	self.params = args
}

type EnumerationOptions struct {
	Choices map[string]string
	Target  string
}

type Enumeration struct {
	*BaseParser
	profile *Profile
	parser  Parser
	options *EnumerationOptions
}

func NewEnumeration(type_name string, profile *Profile) *Enumeration {
	return &Enumeration{&BaseParser{
		type_name: type_name,
	}, profile, nil, &EnumerationOptions{}}
}

func (self Enumeration) Copy() Parser {
	return &self
}

func (self *Enumeration) getParser() (Parser, bool) {
	target := "unsigned int"
	if self.options.Target != "" {
		target = self.options.Target
	}

	parser, ok := self.profile.getParser(target)
	if !ok {
		return nil, false
	}

	self.parser = parser
	return self.parser, true
}

func (self *Enumeration) AsString(offset int64, reader io.ReaderAt) string {
	parser, pres := self.getParser()
	if !pres {
		return ""
	}

	integer, ok := parser.(Integerer)
	if ok {
		string_int := fmt.Sprintf("%d", integer.AsInteger(offset, reader))
		name, pres := self.options.Choices[string_int]
		if pres {
			return name
		}
		return string_int
	}

	return ""
}

func (self *Enumeration) DebugString(offset int64, reader io.ReaderAt) string {
	return self.AsString(offset, reader)
}

func (self *Enumeration) ShortDebugString(offset int64, reader io.ReaderAt) string {
	return self.AsString(offset, reader)
}

func (self *Enumeration) Size(offset int64, reader io.ReaderAt) int64 {
	parser, ok := self.getParser()
	if ok {
		return parser.Size(offset, reader)
	}

	return 0
}

func (self *Enumeration) IsValid(offset int64, reader io.ReaderAt) bool {
	parser, ok := self.getParser()
	if ok {
		return parser.IsValid(offset, reader)
	}

	return false
}

func (self *Enumeration) ParseArgs(args *json.RawMessage) error {
	return json.Unmarshal(*args, &self.options)
}
