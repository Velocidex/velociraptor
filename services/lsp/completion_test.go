package lsp_test

import (
	"fmt"
	"strings"

	"go.lsp.dev/protocol"
	"go.lsp.dev/uri"
	"www.velocidex.com/golang/velociraptor/services/lsp"
	"www.velocidex.com/golang/velociraptor/vtesting/assert"
	"www.velocidex.com/golang/velociraptor/vtesting/goldie"
)

var (
	completionTC = []struct {
		Name   string
		Query  string
		Column uint32
	}{{
		Name:   "Complete VQL function",
		Query:  "SELECT geoip(db='Foo', ip='127.0.0.1') AS Foo FROM scope()",
		Column: 10,
	}, {
		Name:   "Complete VQL plugin",
		Query:  "SELECT * FROM scope()",
		Column: 16,
	}, {
		Name:   "Complete VQL nested function",
		Query:  "SELECT * FROM glob(globs=lowcase(string='*'))",
		Column: 46 - 17,
	}, {
		Name:   "Complete plugin all args",
		Query:  "SELECT * FROM glob()",
		Column: 36 - 17,
	}, {
		Name:   "Complete a plugin args",
		Query:  "SELECT * FROM glob(gloXXX='*')",
		Column: 20,
	}, {
		Name:   "Complete some plugin args (accessor already exists, so should not be completed) ",
		Query:  "SELECT * FROM glob(gloXXX='*', accessor='file')",
		Column: 19,
	}, {
		Name:   "Prefix fallback mid name",
		Query:  "SELECT * FROM parse_j",
		Column: 20,
	}, {
		Name:   "Plugin position after FROM only offers plugins",
		Query:  "select * from pars",
		Column: 18,
	}, {
		Name:   "Unclosed paren completes args of the call before it",
		Query:  "SELECT * FROM pslist(",
		Column: 21,
	}, {
		Name:   "Trigger dot hiding the paren still completes args",
		Query:  "SELECT * FROM pslist(.",
		Column: 22,
	}, {
		Name:   "Symbol callsite falls back to prefix completion",
		Query:  "SELECT parse_ FROM scope()",
		Column: 13,
	}, {
		Name:   "Past end of document",
		Query:  "SELECT * FROM scope()",
		Column: 300,
	}}
)

func (self *LSPTestSuite) TestCompletion() {
	lsp_service := lsp.NewLSPServer(self.ConfigObj).(*lsp.LSPServer)

	var golden []string

	for idx, tc := range completionTC {
		if false && idx != 6 {
			continue
		}

		doc_url := uri.URI(fmt.Sprintf("file:///XXX%d", idx))

		column := int(tc.Column)
		if column > len(tc.Query) {
			column = len(tc.Query)
		}

		golden = append(golden, fmt.Sprintf(
			"\nTest case %d: %s\n%v\n%v<--",
			idx, tc.Name, tc.Query, tc.Query[:column]))

		// Load the document.
		diagnostics, err := lsp_service.DidOpen(self.Ctx,
			&protocol.DidOpenTextDocumentParams{
				TextDocument: protocol.TextDocumentItem{
					URI:  doc_url,
					Text: tc.Query,
				},
			})
		assert.NoError(self.T(), err)

		// No issue with the VqL
		golden = append(golden, "Diagnostics:")
		golden = append(golden, lsp.DumpProtool(diagnostics))

		req := &protocol.CompletionParams{
			TextDocumentPositionParams: protocol.TextDocumentPositionParams{
				TextDocument: protocol.TextDocumentIdentifier{
					URI: doc_url,
				},
				Position: protocol.Position{
					Line:      0,
					Character: tc.Column,
				},
			},
		}

		// Now get completion for function name
		completions, err := lsp_service.Completion(self.Ctx, req)
		assert.NoError(self.T(), err)

		golden = append(golden, "Completions:")
		golden = append(golden, lsp.DumpProtool(completions))
		golden = append(golden, "text edit:")
		for _, c := range completions {
			c_ := c.TextEdit.(*protocol.TextEdit)
			golden = append(golden,
				fmt.Sprintf("%v -> %v", getRange(tc.Query, c_.Range), c_.NewText))
		}
	}

	goldie.Assert(self.T(), "TestCompletion",
		[]byte(strings.Join(golden, "\n")))
}

// Extracts the substring from the text specified by the exclusive
// range.
func getRange(text string, rng protocol.Range) string {
	start := getPos(text, rng.Start)
	end := getPos(text, rng.End)

	if end > len(text) {
		end = len(text)
	}
	if start >= end {
		return ""
	}
	return text[start:end]
}

func getPos(text string, pos protocol.Position) int {
	var cursor protocol.Position

	for idx, c := range text {
		if cursor.Line == pos.Line && cursor.Character == pos.Character {
			return idx
		}

		if c == '\n' {
			cursor.Line++
			cursor.Character = 0
		} else {
			cursor.Character++
		}
	}
	return len(text)
}
