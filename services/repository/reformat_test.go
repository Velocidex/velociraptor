package repository

import (
	"strings"
	"testing"

	"github.com/Velocidex/ordereddict"
	"github.com/stretchr/testify/assert"
	"www.velocidex.com/golang/velociraptor/vtesting/goldie"
)

type reformatCases_t struct {
	name    string
	in, out string
}

var reformatCases = []reformatCases_t{
	{name: "Simple",
		in: `
name: Foo
sources:
- query: |
    SELECT A,B,C
    FROM info(arg1=Foobar, arg2="XXXXX")
- query: |-
    SELECT A,B,C      FROM info()
- description: Foo bar
`}, {
		name: "Single line queries are not reformatted.",
		in: `
name: Single line
sources:
- query: SELECT * FROM info()
- query: |
    SELECT A FROM scope()
`}, {
		name: "export reformatted",
		in: `
name: Exported
export: |
  SELECT * FROM scope()
`}, {
		name: "preconditions",
		in: `
name: Preconditions
precondition: SELECT * FROM info()
sources:
- precondition: |
   SELECT * FROM info()
`}, {
		name: "notebook",
		in: `
name: Notebook
sources:
- query: |
    SELECT A,B,C
    FROM scope()
  notebook:
    - name: Test
      type: vql_suggestion
      template: |
        SELECT * FROM scope()
`}, {
		name: "column types",
		in: `
name: Column Types
sources:
- query: |
    SELECT A,B,C
    FROM scope()
column_types:
  - name: A
    type: string
`},
}

func TestReformat(t *testing.T) {
	golden := ordereddict.NewDict()
	for _, c := range reformatCases {
		out, err := reformatVQL(c.in)
		assert.NoError(t, err)
		golden.Set(c.name, strings.Split(out, "\n"))
	}
	goldie.AssertJson(t, "TestReformat", golden)
}

// Test that when VQL is reformatted multiple times it doesn't change.
func TestReformatMultiple(t *testing.T) {
	golden := ordereddict.NewDict()
	for _, c := range reformatCases {
		first, err := reformatVQL(c.in)
		second, err := reformatVQL(first)
		assert.NoError(t, err)
		golden.Set(c.name, strings.Split(second, "\n"))
	}
	goldie.AssertJson(t, "TestReformat", golden)
}
