package reporting

import (
	"fmt"
	"strings"

	"github.com/Velocidex/ordereddict"
	"github.com/alecthomas/participle"
	"github.com/alecthomas/participle/lexer"
	"github.com/alecthomas/participle/lexer/stateful"
)

var (
	def = lexer.Must(stateful.New(stateful.Rules{
		"Root": {
			{`VQLText`, `(?ms)([^/]|/[^*])+`, nil},
			{"CommentStart", `(?ms)/[*]`, stateful.Push("Comment")},
		},
		"Comment": {
			{`CommentText`, `(?ms)([^*]|[*][^/])+`, nil},
			{"CommentEnd", `(?ms)[*]/`, stateful.Pop()},
		},
	}))
	parser = participle.MustBuild(&Content{}, participle.Lexer(def))
)

type Fragment struct {
	VQL     string `(  @VQLText  |`
	Comment string ` CommentStart @CommentText CommentEnd )`
}

type Content struct {
	Fragments []Fragment ` @@* `
}

func parseVQLCell(content string) (*Content, error) {
	result := &Content{}
	err := parser.ParseString(content, result)
	return result, err
}

// Convert the VQL cell into a template that produces markdown
// cells. VQL cells may interleave Markdown within them.
func ConvertVQLCellToMarkdownCell(content string) (string, *ordereddict.Dict) {
	env := ordereddict.NewDict()

	cell_content, err := parseVQLCell(content)
	if err != nil {
		return fmt.Sprintf("# Error: %v", err), env
	}

	result := ""
	for idx, fragment := range cell_content.Fragments {
		vql := strings.TrimSpace(fragment.VQL)
		if vql != "" {
			key := fmt.Sprintf("Query%v", idx)
			env.Set(key, vql)
			result += fmt.Sprintf(
				"\n{{ Query \"SELECT * FROM query(query=%s)\" | Table }}\n", key)
			continue
		}

		lines := strings.SplitN(fragment.Comment, "\n", 2)
		if len(lines) == 1 {
			result += lines[0]
		} else {
			result += lines[1]
		}
	}

	return result, env
}
