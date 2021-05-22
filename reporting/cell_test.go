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

*/

SELECT * FROM glob()
`},
}

func TestVQL2MarkdownConversion(t *testing.T) {
	result := ordereddict.NewDict()
	for _, testcase := range TestCases {
		parsed, err := parseVQLCell(testcase.Input)
		assert.NoError(t, err)

		result.Set(testcase.Name, parsed)

		content, env := ConvertVQLCellToMarkdownCell(testcase.Input)
		result.Set(testcase.Name+" Markdown", content)
		result.Set(testcase.Name+" Markdown Env", env)
	}

	g := goldie.New(
		t,
		goldie.WithFixtureDir("fixtures"),
		goldie.WithNameSuffix(".golden"),
		goldie.WithDiffEngine(goldie.ColoredDiff),
	)
	g.AssertJson(t, "VQL2MarkdownConversion", result)
}
