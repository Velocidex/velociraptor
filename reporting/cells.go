package reporting

import (
	"github.com/alecthomas/participle"
	"github.com/alecthomas/participle/lexer"
	"github.com/alecthomas/participle/lexer/stateful"
)

var (
	def = lexer.Must(stateful.New(stateful.Rules{
		"Root": {
			{"CommentStart", `^ */[*]`, stateful.Push("Comment")},
			{`VQLText`, `(?ms)^([^\n]+|\n)`, nil},
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

func (self *Content) PushVQL(vql string) {
	last_idx := len(self.Fragments) - 1

	if len(self.Fragments) == 0 || self.Fragments[last_idx].VQL == "" {
		self.Fragments = append(self.Fragments, Fragment{VQL: vql})
	} else {
		self.Fragments[last_idx].VQL += vql
	}
}

func (self *Content) PushComment(c string) {
	last_idx := len(self.Fragments) - 1

	if len(self.Fragments) == 0 || self.Fragments[last_idx].Comment == "" {
		self.Fragments = append(self.Fragments, Fragment{Comment: c})
	} else {
		self.Fragments[last_idx].Comment += c
	}
}

// A VQL cell consists of an interleaved set of markdown and VQL.
func ConvertVQLCellToContent(content string) (*Content, error) {
	parsed := &Content{}
	err := parser.ParseString(content, parsed)
	if err != nil {
		return nil, err
	}

	result := &Content{}
	for _, fragment := range parsed.Fragments {
		if fragment.VQL != "" {
			result.PushVQL(fragment.VQL)
		} else if fragment.Comment != "" {
			result.PushComment(fragment.Comment)
		}
	}
	return result, err
}
