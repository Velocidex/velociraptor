package vql_test

import (
	"context"
	"os"
	"os/exec"
	"testing"
	"time"

	"www.velocidex.com/golang/velociraptor/json"
	"www.velocidex.com/golang/velociraptor/vtesting/assert"
	"www.velocidex.com/golang/vfilter"
)

// Materializing a query argument with more than 10,000 rows used to
// recurse while logging the warning, so the query never completed.
const storedQueryArgLargeTest = `
LET F(Rows) = SELECT * FROM Rows
SELECT * FROM F(Rows={ SELECT * FROM range(end=10001) }) LIMIT 1
`

func evalVQL(t *testing.T, query string) []string {
	t.Helper()

	statements, err := vfilter.MultiParse(query)
	assert.NoError(t, err)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	scope := vfilter.NewScope()
	defer scope.Close()

	result := []string{}
	for _, statement := range statements {
		for row := range statement.Eval(ctx, scope) {
			result = append(result, json.MustMarshalString(row))
		}
	}
	return result
}

// Run the query in a child process: the old behaviour recurses until
// the stack overflows, which would otherwise take down the whole test
// binary.
func TestStoredQueryArgLargeResult(t *testing.T) {
	const childEnv = "VELOCIRAPTOR_TEST_STORED_QUERY_ARG"

	if os.Getenv(childEnv) == "1" {
		assert.Equal(t, []string{`{"_value":0}`}, evalVQL(t, storedQueryArgLargeTest))
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, os.Args[0],
		"-test.run=^TestStoredQueryArgLargeResult$")
	cmd.Env = append(os.Environ(), childEnv+"=1")
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("query did not complete: %v\n%s", err, output)
	}
}
