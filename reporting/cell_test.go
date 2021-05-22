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
