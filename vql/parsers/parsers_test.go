package parsers_test

import (
	"context"
	"fmt"
	"log"
	"os"
	"testing"

	"github.com/Velocidex/ordereddict"
	"github.com/stretchr/testify/suite"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	"www.velocidex.com/golang/velociraptor/json"
	"www.velocidex.com/golang/velociraptor/utils"
	vql_subsystem "www.velocidex.com/golang/velociraptor/vql"
	"www.velocidex.com/golang/velociraptor/vql/functions"
	"www.velocidex.com/golang/velociraptor/vql/parsers"
	"www.velocidex.com/golang/velociraptor/vtesting/goldie"
	"www.velocidex.com/golang/vfilter"

	_ "www.velocidex.com/golang/velociraptor/accessors/data"
)

type ParserTestSuite struct {
	suite.Suite
}

var yamlTestCases = []string{`
Name: myname
Value:
  - list1
  - list2
`, `
# Test that we maintain the order of keys in yaml documents.
Name: myname1
B: 1
A: 2
c:
  Field1: X
  Field2: Y
  Field0: Z
`, `
# A multiline plain scalar.
Field1
  field2
`}

func (self *ParserTestSuite) TestYamlParser() {
	result := ordereddict.NewDict()
	ctx := context.Background()
	scope := vql_subsystem.MakeScope()
	scope.SetLogger(log.New(os.Stderr, "", 0))

	defer scope.Close()

	for idx, item := range yamlTestCases {
		value := parsers.ParseYamlFunction{}.Call(ctx, scope, ordereddict.NewDict().
			Set("filename", item).
			Set("accessor", "data"))
		result.Set(fmt.Sprintf("Case %v", idx), value)
	}
	goldie.Assert(self.T(), "TestYamlParser", json.MustMarshalIndent(result))
}

func TestParsers(t *testing.T) {
	suite.Run(t, &ParserTestSuite{})
}

func (self *ParserTestSuite) TestYAMLSerializeRoundTrip() {
	ctx := context.Background()
	scope := vql_subsystem.MakeScope()
	defer scope.Close()
	parse := func(text string) interface{} {
		return parsers.ParseYamlFunction{}.Call(ctx, scope, ordereddict.NewDict().
			Set("filename", text).Set("accessor", "data"))
	}
	input := "z: Y\nrows:\n- z: first\n  a: second\n"
	value := parse(input)
	self.Require().NotNil(value)
	output := (&functions.EncodeFunction{}).Call(ctx, scope, ordereddict.NewDict().
		Set("item", value).Set("format", "yaml"))
	self.Require().IsType("", output)
	self.Contains(output, "rows:\n- z: first\n  a: second\n")
	self.Equal(string(json.MustMarshalIndent(value)),
		string(json.MustMarshalIndent(parse(output.(string)))))
	self.Nil(parse("rows: ["))
	self.Nil(parse("rows: [!!int invalid]"))
	self.Nil(parse("value: &self [*self]"))
	self.Nil(parse("dup: 1\ndup: 2\n"))

	merged := parse("base: &base\n  z: first\n  a: second\ncopy:\n  <<: *base\n  b: third\nalias_ref: *base\n")
	self.Require().NotNil(merged)
	self.Equal("{\n \"base\": {\n  \"z\": \"first\",\n  \"a\": \"second\"\n },\n \"copy\": {\n  \"z\": \"first\",\n  \"a\": \"second\",\n  \"b\": \"third\"\n },\n \"alias_ref\": {\n  \"z\": \"first\",\n  \"a\": \"second\"\n }\n}",
		string(json.MustMarshalIndent(merged)))

	cyclic := ordereddict.NewDict()
	cyclic.Set("self", cyclic)
	self.Equal(vfilter.Null{}, (&functions.EncodeFunction{}).Call(ctx, scope, ordereddict.NewDict().
		Set("item", cyclic).Set("format", "yaml")))
	for _, test := range []struct {
		text string
		want bool
	}{{"Y", true}, {"N", false}} {
		out := (&functions.EncodeFunction{}).Call(ctx, scope, ordereddict.NewDict().
			Set("item", parse("lockdown: "+test.text)).Set("format", "yaml"))
		self.Require().IsType("", out)
		config := &config_proto.Config{}
		self.Require().NoError(utils.YamlUnmarshalStrict([]byte(out.(string)), config))
		self.Equal(test.want, config.Lockdown)
	}
}
