package reporting

import (
	"testing"

	"github.com/Velocidex/ordereddict"
	"github.com/alecthomas/assert"
	"github.com/sebdah/goldie/v2"
)

type testCases struct {
	Name, Input string
}

var TestCases = []testCases{
	{"Markdown in comments", `
SELECT 1 / 2 FROM info()

/* c

# This is markdown
with multiple lines
/*
This is still a comment!

*/

SELECT * FROM glob()
`}, {"Comments in VQL", `
SELECT * FROM glob(globs='C:/Windows/*/*.exe')
`}, {"Comments within lines", `
SELECT * FROM glob(/* Foobar */globs='C:/Windows/*/*.exe')
`}, {"Multi line VQL", `
SELECT *, count() AS Count
FROM certificates()
`}, {"Empty lines before comment", `



/* This is a comment */

SELECT * FROM info()

`}, {"Whitespace between query", `

SELECT
*
FROM
info()
`}, {"Comment as first line", `
-- hello
SELECT * FROM info()
`},
}

func TestVQL2MarkdownConversion(t *testing.T) {
	result := ordereddict.NewDict()
	for _, testcase := range TestCases {
		/*
			tokens, err := parser.Lex(bytes.NewReader([]byte(testcase.Input)))
			utils.Debug(tokens)
			utils.Debug(err)
		*/
		parsed, err := ConvertVQLCellToContent(testcase.Input)
		assert.NoError(t, err)

		result.Set(testcase.Name, parsed)
	}

	g := goldie.New(
		t,
		goldie.WithFixtureDir("fixtures"),
		goldie.WithNameSuffix(".golden"),
		goldie.WithDiffEngine(goldie.ColoredDiff),
	)
	g.AssertJson(t, "VQL2MarkdownConversion", result)
}
