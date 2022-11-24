// +build cgo,yara

package common

import (
	"context"
	"log"
	"os"
	"testing"

	"github.com/Velocidex/ordereddict"
	"github.com/sebdah/goldie"
	"github.com/stretchr/testify/suite"
	"www.velocidex.com/golang/velociraptor/file_store/test_utils"
	"www.velocidex.com/golang/velociraptor/json"
	vql_subsystem "www.velocidex.com/golang/velociraptor/vql"
	"www.velocidex.com/golang/vfilter/types"

	_ "www.velocidex.com/golang/velociraptor/accessors/data"
)

type YaraTestSuite struct {
	test_utils.TestSuite
}

type testCase struct {
	description, rule, data string
}

var yaraTestCases = []testCase{
	{
		description: "Match simple string",
		rule: `
rule X {
  meta:
    foobar = 23
    name = "hello me"
  strings:
     $a = "hello" nocase ascii wide
  condition: any of them
}`,
		data: "Hello world",
	},
}

func (self *YaraTestSuite) TestCSVParser() {
	result := ordereddict.NewDict()
	ctx := context.Background()
	scope := vql_subsystem.MakeScope()
	scope.SetLogger(log.New(os.Stderr, "", 0))

	defer scope.Close()

	plugin := YaraScanPlugin{}
	for _, test_case := range yaraTestCases {
		rows := []types.Row{}
		args := ordereddict.NewDict().
			Set("rules", test_case.rule).
			Set("files", test_case.data).
			Set("accessor", "data")

		for row := range plugin.Call(ctx, scope, args) {
			rows = append(rows, row)
		}

		result.Set(test_case.description, rows)
	}
	goldie.Assert(self.T(), "TestYara", json.MustMarshalIndent(result))
}

func TestYara(t *testing.T) {
	suite.Run(t, &YaraTestSuite{})
}
