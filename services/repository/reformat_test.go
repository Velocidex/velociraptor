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
