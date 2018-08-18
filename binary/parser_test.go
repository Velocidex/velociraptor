//
package binary

import (
	"bytes"
	"fmt"
	assert "github.com/stretchr/testify/assert"
	"testing"
)

var (
	sample = []byte{0x01, 0x02, 0x03, 0x04, 0x05, 0x06, 0x07, 0x08,
		0x09, 0x0a, 0x0b, 0x0c, 0x0d, 0x0e, 0x0f, 0x10, 0x11, 0x12, 0x13}
)

func TestIntegerParser(t *testing.T) {
	reader := bytes.NewReader(sample)
	profile := NewProfile()
	AddModel(profile)

	base_obj := BaseObject{
		reader: reader,
		offset: 0,
		parser: profile.types["unsigned long long"],
	}
	assert.Equal(t, uint64(0x0807060504030201), base_obj.AsInteger())
}

func TestStructParser(t *testing.T) {
	reader := bytes.NewReader(sample)
	profile := NewProfile()
	AddModel(profile)

	struct_parser := NewStructParser("TestStruct", 10)
	struct_parser.AddParser("Field1",
		&ParseAtOffset{
			offset: 2,
			name:   "",
			parser: profile.types["unsigned long long"],
		})

	base_obj := BaseObject{
		reader: reader,
		offset: 0,
		parser: struct_parser,
	}

	assert.IsType(t,
		&ErrorObject{},
		base_obj.Get("NoSuchField"))

	assert.IsType(t,
		&ErrorObject{},
		base_obj.Get("NoSuchField").Get("FooBar"))

	assert.Equal(t,
		uint64(0x0a09080706050403),
		base_obj.Get("Field1").AsInteger())

}

func TestNestedStructParser(t *testing.T) {
	reader := bytes.NewReader(sample)
	profile := NewProfile()
	AddModel(profile)

	struct_parser := NewStructParser("NestedStruct", 10)
	struct_parser.AddParser("NestedField",
		&ParseAtOffset{
			offset: 2,
			name:   "",
			parser: profile.types["unsigned short"]})

	struct_parser_2 := NewStructParser("TestStruct", 10)
	struct_parser_2.AddParser("Field1",
		&ParseAtOffset{
			offset: 2,
			name:   "",
			parser: struct_parser})

	base_obj := BaseObject{
		reader: reader,
		offset: 0,
		parser: struct_parser_2,
	}

	assert.Equal(t,
		uint64(0x605),
		base_obj.Get("Field1").Get("NestedField").AsInteger())

	assert.Equal(t,
		uint64(0x605),
		base_obj.Get("Field1.NestedField").AsInteger())
}

func TestUnpacking(t *testing.T) {
	definition := `
{
  "TestStruct": [10, {
     "Field1": [2, ["unsigned long long", {}]],
     "Field2": [4, ["Second"]]
  }],
  "Second": [5, {
     "SecondF1": [2, ["unsigned long long"]]
  }]
}
`
	reader := bytes.NewReader(sample)
	profile := NewProfile()
	AddModel(profile)

	err := profile.ParseStructDefinitions(definition)
	if err != nil {
		t.Fatalf(err.Error())
	}

	test_struct, err := profile.Create("TestStruct", 2, reader, nil)
	assert.NoError(t, err)

	assert.Equal(t, uint64(0x0c0b0a0908070605),
		test_struct.Get("Field1").AsInteger())

	fmt.Println(test_struct.DebugString())
}
