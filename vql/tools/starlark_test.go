package tools

import (
	"context"
	"fmt"
	"log"
	"os"
	"testing"

	"github.com/Velocidex/ordereddict"
	"github.com/sebdah/goldie"
	"github.com/stretchr/testify/suite"
	"www.velocidex.com/golang/velociraptor/json"
	vql_subsystem "www.velocidex.com/golang/velociraptor/vql"
	"www.velocidex.com/golang/vfilter"
	"www.velocidex.com/golang/vfilter/types"
)

type testCase struct {
	name string
	vql  string
	star string
}

var starlarkTestCases = []testCase{
	{`Starlark Module Materialized`, `
LET X <= starl(code=StarCode)
SELECT X.Foo(X=2), X.SomeInt, X.Foo, X.SomeInt(X=1) FROM scope()
`, `
SomeInt = 2
def Foo(X):
  return X + 2
`},

	{`Starlark Module`, `
LET X = starl(code=StarCode)
SELECT X.Foo(X=2), X.SomeInt, X.Foo, X.SomeInt(X=1) FROM scope()
`, `
SomeInt = 2
def Foo(X):
  return X + 2
`},
	{`Starlark types`, `
LET X = starl(code=StarCode)
SELECT X.Foo(X=2, Y="String", Z=[2, 3]) FROM scope()
`, `
def Foo(X, Y, Z):
   return X + 2, Y + "A", [1,] + Z
`},
}

type StarlarkTestSuite struct {
	suite.Suite
}

func (self *StarlarkTestSuite) TestStarlarkFunc() {
	result := ordereddict.NewDict()
	ctx := context.Background()

	for i, test_case := range starlarkTestCases {
		scope := vql_subsystem.MakeScope()
		scope.SetLogger(log.New(os.Stderr, "", 0))

		defer scope.Close()

		scope.AppendVars(ordereddict.NewDict().Set("StarCode", test_case.star))

		multi_vql, err := vfilter.MultiParse(test_case.vql)
		if err != nil {
			self.T().Fatalf("Failed to parse %v: %v", test_case.vql, err)
		}

		for idx, vql := range multi_vql {
			var output []types.Row

			for row := range vql.Eval(ctx, scope) {
				output = append(output, vfilter.RowToDict(ctx, scope, row))
			}

			result.Set(fmt.Sprintf("%03d/%03d %s: %s", i, idx, test_case.name,
				vql.ToString(scope)), output)
		}

	}
	goldie.Assert(self.T(), "TestStarlark", json.MustMarshalIndent(result))
}

func TestStartlarkPlugin(t *testing.T) {
	suite.Run(t, &StarlarkTestSuite{})
}
