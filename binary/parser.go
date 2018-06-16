// Implements a binary parsing system.
package binary

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"sort"
	"strings"
	//	utils "www.velocidex.com/golang/velociraptor/testing"
)

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

type Object interface {
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
}

type BaseObject struct {
	reader    io.ReaderAt
	offset    int64
	name      string
	type_name string
	parser    Parser
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

	switch self.parser.(type) {
	case Getter:
		return self.parser.(Getter).Get(self, field)
	default:
		return NewErrorObject("Parser does not support Get")
	}
}

func (self *BaseObject) DebugString() string {
	return self.parser.DebugString(self.offset, self.reader)
}

func (self *BaseObject) Size() int64 {
	return self.parser.Size(self.offset, self.reader)
}

func (self *BaseObject) IsValid() bool {
	buf := make([]byte, 8)
	_, err := self.reader.ReadAt(buf, self.offset+self.Size())
	if err != nil {
		return false
	}

	return true
}

func (self *BaseObject) Value() interface{} {
	switch self.parser.(type) {
	case Stringer:
		return self.AsString()
	case Integerer:
		return self.AsInteger()
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

type ErrorObject struct {
	message string
}

func NewErrorObject(message string) *ErrorObject {
	return &ErrorObject{message}
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

type IntParser struct {
	type_name string
	name      string
	size      int64
	converter func(buf []byte) uint64
}

func (self *IntParser) Copy() Parser {
	result := *self
	return &result
}

func (self *IntParser) SetName(name string) {
	self.name = name
}

func (self *IntParser) AsInteger(offset int64, reader io.ReaderAt) uint64 {
	buf := make([]byte, 8)

	_, err := reader.ReadAt(buf, offset)
	if err != nil {
		return 0
	}

	return self.converter(buf)
}

func (self *IntParser) DebugString(offset int64, reader io.ReaderAt) string {
	return fmt.Sprintf("[%s] %#0x",
		self.type_name, self.AsInteger(offset, reader))
}

func (self *IntParser) ShortDebugString(offset int64, reader io.ReaderAt) string {
	return fmt.Sprintf("%#0x", self.AsInteger(offset, reader))
}

func (self *IntParser) Size(offset int64, reader io.ReaderAt) int64 {
	return self.size
}

func (self *IntParser) IsValid(offset int64, reader io.ReaderAt) bool {
	buf := make([]byte, 8)

	_, err := reader.ReadAt(buf, offset)
	if err != nil {
		return false
	}
	return true
}

func (self *IntParser) ParseArgs(args *json.RawMessage) error {
	return nil
}

type StringParser struct {
	type_name string
	Name      string
	options   struct {
		Length int64
	}
}

func (self *StringParser) Copy() Parser {
	result := *self
	return &result
}

func (self *StringParser) SetName(name string) {
	self.Name = name
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

func (self *StringParser) IsValid(offset int64, reader io.ReaderAt) bool {
	buf := make([]byte, 8)

	_, err := reader.ReadAt(buf, offset)
	if err != nil {
		return false
	}
	return true
}

func (self *StringParser) DebugString(offset int64, reader io.ReaderAt) string {
	return self.AsString(offset, reader)
}

func (self *StringParser) ShortDebugString(offset int64, reader io.ReaderAt) string {
	return self.AsString(offset, reader)
}

func (self *StringParser) ParseArgs(args *json.RawMessage) error {
	err := json.Unmarshal(*args, &self.options)
	if err != nil {
		return err
	}
	return nil
}

type StructParser struct {
	size      int64
	type_name string
	Name      string
	fields    map[string]Parser
}

func (self *StructParser) Copy() Parser {
	result := *self
	return &result
}

func (self *StructParser) SetName(name string) {
	self.Name = name
}

func (self *StructParser) Get(base Object, field string) Object {
	parser, pres := self.fields[field]
	if pres {
		getter, ok := parser.(Getter)
		if ok {
			return getter.Get(base, field)
		}
	}

	return NewErrorObject("Field not known.")
}

func (self *StructParser) Size(offset int64, reader io.ReaderAt) int64 {
	return self.size
}

func (self *StructParser) IsValid(offset int64, reader io.ReaderAt) bool {
	buf := make([]byte, 8)
	_, err := reader.ReadAt(buf, offset+self.size)
	if err != nil {
		return false
	}

	return true
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

func (self *StructParser) AddParser(field string, parser Parser) {
	self.fields[field] = parser
}

func (self *StructParser) ParseArgs(args *json.RawMessage) error {
	return nil
}

func NewStructParser(type_name string, size int64) *StructParser {
	result := StructParser{
		size:      size,
		Name:      type_name,
		type_name: type_name,
		fields:    make(map[string]Parser)}

	return &result
}

type ParseAtOffset struct {
	offset    int64
	name      string
	type_name string
	profile   *Profile
	parser    Parser
	args      *json.RawMessage
}

func (self *ParseAtOffset) Copy() Parser {
	result := *self
	return &result
}

func (self *ParseAtOffset) Get(base Object, field string) Object {
	parser, pres := self.getParser(self.type_name)
	if !pres {
		return &ErrorObject{"Type not found"}
	}

	return &BaseObject{
		offset: base.Offset() + self.offset,
		reader: base.Reader(),
		parser: parser}
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
		return "Type not found"
	}

	return fmt.Sprintf(
		"%#03x  %s  %s", self.offset, self.name,
		parser.ShortDebugString(self.offset+offset, reader))
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

	self.parser = parser.Copy()
	return self.parser, true
}

func (self *ParseAtOffset) ParseArgs(args *json.RawMessage) error {
	parser, ok := self.getParser(self.type_name)
	if ok {
		return parser.ParseArgs(args)
	}
	return nil
}

type Enumeration struct {
	type_name string
	Name      string
	profile   *Profile
	parser    Parser
	options   struct {
		Choices map[string]string
		Target  string
	}
}

func (self *Enumeration) Copy() Parser {
	result := *self
	return &result
}

func (self *Enumeration) SetName(name string) {
	self.Name = name
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

	self.parser = parser.Copy()
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
	err := json.Unmarshal(*args, &self.options)
	if err != nil {
		return err
	}
	return nil
}
