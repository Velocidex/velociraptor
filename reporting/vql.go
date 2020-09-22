//+build !codeanalysis

package reporting

import (
	. "github.com/alecthomas/chroma" // nolint
	"github.com/alecthomas/chroma/lexers"
)

// VQL lexer for syntax highlighting.
var VQL = lexers.Register(MustNewLexer(
	&Config{
		Name:            "VQL",
		Aliases:         []string{"vql"},
		Filenames:       []string{"*.vql"},
		MimeTypes:       []string{"text/x-vql"},
		NotMultiline:    true,
		CaseInsensitive: true,
	},
	Rules{
		"root": {
			{`\s+`, Text, nil},
			{`--.*\n?`, CommentSingle, nil},
			{`//(\n|[\w\W]*?[^\\]\n)`, CommentSingle, nil},
			{`/\*`, CommentMultiline, Push("multiline-comments")},
			{`'`, LiteralStringSingle, Push("string")},
			{`"`, LiteralStringDouble, Push("double-string")},
			{`(AS)(\b\s+)([a-z0-9_]+)`, ByGroups(
				Keyword, Text, NameDecorator), nil},
			{Words(``, `\b`, `LET`), Keyword, Push("let")},
			{Words(``, `\b`, `SELECT`, `FROM`, `WHERE`,
				`GROUP`, `BY`, `ORDER`, `LIMIT`), Keyword, nil},
			{"[+*/<>=~!@#%^&|`?-]", Operator, nil},
			{`([a-z_][\w$]*)(=)`, ByGroups(NameTag, Operator), nil},
			{`([a-z_][.\w$]*)(\()`, ByGroups(NameFunction, Operator), nil},
			{`[a-z_][\w$]*`, NameVariable, nil},
			{`[;:()\[\],.]`, Operator, nil},
			{`[0-9.]+`, LiteralNumber, nil},
			{`[{}]`, Operator, nil},
			{`(true|false|NULL)\b`, NameBuiltin, nil},
		},
		"let": {
			{`\s+`, Text, nil},
			{`[a-zA-Z_0-9]`, NameVariable, nil},
			{`(=|<=)`, Operator, Pop(1)},
		},
		"multiline-comments": {
			{`/\*`, CommentMultiline, Push("multiline-comments")},
			{`\*/`, CommentMultiline, Pop(1)},
			{`[^/*]+`, CommentMultiline, nil},
			{`[/*]`, CommentMultiline, nil},
		},
		"string": {
			{`[^']+`, LiteralStringSingle, nil},
			{`''`, LiteralStringSingle, nil},
			{`'`, LiteralStringSingle, Pop(1)},
		},
		"double-string": {
			{`[^"]+`, LiteralStringDouble, nil},
			{`""`, LiteralStringDouble, nil},
			{`"`, LiteralStringDouble, Pop(1)},
		},
	},
))
