package main

import (
	"context"
	"fmt"
	"io/ioutil"
	"os"
	"strings"
	"testing"

	"github.com/Velocidex/ordereddict"
	"github.com/stretchr/testify/assert"
	"www.velocidex.com/golang/velociraptor/utils/tempfile"
	vql_subsystem "www.velocidex.com/golang/velociraptor/vql"
	"www.velocidex.com/golang/velociraptor/vtesting/goldie"
	"www.velocidex.com/golang/vfilter"
)

var marshalTestCases = []struct {
	name     string
	pre_vql  string
	post_vql string
}{
	{`Starlark function`,
		`LET X <= starl(code='def foo(x): return x+1')`,
		`SELECT X.foo(x=1) FROM scope()`,
	},
}

func TestMarshal(t *testing.T) {
	tmpfile, err := tempfile.TempFile("")
	assert.NoError(t, err)
	tmpfile.Close()

	defer os.Remove(tmpfile.Name())

	results := ordereddict.NewDict()

	for idx, testCase := range marshalTestCases {
		scope := vql_subsystem.MakeScope()
		multi_vql, err := vfilter.MultiParse(testCase.pre_vql)
		if err != nil {
			t.Fatalf("Failed to parse %v: %v", testCase.pre_vql, err)
		}

		// Ignore the rows returns in the pre_vql - it is just
		// used to set up the scope.
		ctx := context.Background()
		for _, vql := range multi_vql {
			for _ = range vql.Eval(ctx, scope) {
			}
		}

		// Serialize the scope into the tmpfile.
		err = storeScopeInFile(tmpfile.Name(), scope)
		assert.NoError(t, err)

		fd, err := os.Open(tmpfile.Name())
		assert.NoError(t, err)

		data, err := ioutil.ReadAll(fd)
		assert.NoError(t, err)

		results.Set(fmt.Sprintf("%v: Marshal %v", idx, testCase.name),
			strings.Split(string(data), "\n"))

		// Load a new scope from the file.
		new_scope := vql_subsystem.MakeScope()
		new_scope, err = loadScopeFromFile(tmpfile.Name(), new_scope)
		assert.NoError(t, err)

		multi_vql, err = vfilter.MultiParse(testCase.post_vql)
		if err != nil {
			t.Fatalf("Failed to parse %v: %v", testCase.post_vql, err)
		}

		rows := make([]vfilter.Row, 0)
		for _, vql := range multi_vql {
			for row := range vql.Eval(ctx, new_scope) {
				rows = append(rows, row)
			}
		}

		results.Set(fmt.Sprintf("%v: Rows %v", idx, testCase.name), rows)
	}

	goldie.AssertJson(t, "Serialization", results)
}
