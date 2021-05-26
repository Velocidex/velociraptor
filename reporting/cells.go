package reporting

import (
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

// A VQL cell consists of an interleaved set of markdown and VQL.
func ConvertVQLCellToContent(content string) (*Content, error) {
	result := &Content{}
	err := parser.ParseString(content, result)
	return result, err
}
