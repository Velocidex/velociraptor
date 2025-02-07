//go:build cgo && yara
// +build cgo,yara

package common

import (
	"context"
	"log"
	"os"
	"strings"
	"testing"

	"github.com/Velocidex/ordereddict"
	"github.com/stretchr/testify/suite"
	"www.velocidex.com/golang/velociraptor/file_store/test_utils"
	"www.velocidex.com/golang/velociraptor/json"
	vql_subsystem "www.velocidex.com/golang/velociraptor/vql"
	"www.velocidex.com/golang/velociraptor/vtesting/assert"
	"www.velocidex.com/golang/velociraptor/vtesting/goldie"
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

func (self *YaraTestSuite) TestYaraLinter() {
	rule := `
import "pe"

rule TestIssuer {
    condition:
        for any i in (0..pe.number_of_signatures) : ( pe.signatures[i].issuer contains "DigiCert Trusted G4 Code Signing RSA4096 SHA384 2021 CA1" )
}

rule TestPE {
    meta:
      comment = "Some useless metadata"

    strings:
      $a = "Hello"

    condition:
      pe.is_pe
}

rule UnimportedModule {
   condition:
      time.now > 0
}

rule AllOfThem {
   strings:
     $a = "hello"

   condition:
     any of them
}

rule PECondition {
   strings:
    $a = "hello"

   condition:
      pe.is_pe and for any s in pe.sections : ( s.name == ".text" ) and all of them
}
`
	linter, err := NewRuleLinter(rule)
	assert.NoError(self.T(), err)

	linter.ClearMetadata = true

	clean, errors := linter.Lint()
	var err_str []string
	for _, err := range errors {
		err_str = append(err_str, err.Error())
	}

	result := ordereddict.NewDict().
		Set("CleanedRules", strings.Split(clean.String(), "\n")).
		Set("Errors", err_str)

	goldie.Assert(self.T(), "TestYaraLinter",
		json.MustMarshalIndent(result))

}

func TestYara(t *testing.T) {
	suite.Run(t, &YaraTestSuite{})
}
