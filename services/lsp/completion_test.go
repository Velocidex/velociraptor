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
		Column: 18,
	}, {
		Name:   "Complete a plugin args",
		Query:  "SELECT * FROM glob(gloXXX='*')",
		Column: 20,
	}, {
		Name:   "Complete some plugin args (accessor already exists, so should not be completed) ",
		Query:  "SELECT * FROM glob(gloXXX='*', accessor='file')",
		Column: 18,
	}, {
		Name:   "Prefix fallback after trigger dot",
		Query:  "SELECT * FROM parse.",
		Column: 19,
	}, {
		Name:   "Prefix fallback mid name",
		Query:  "SELECT * FROM parse_j",
		Column: 20,
	}}
)

func (self *LSPTestSuite) TestCompletion() {
	lsp_service := lsp.NewLSPServer(self.ConfigObj).(*lsp.LSPServer)

	var golden []string

	for idx, tc := range completionTC {
		if false && idx != 4 {
			continue
		}

		doc := uri.URI(fmt.Sprintf("file:///XXX%d", idx))

		golden = append(golden, fmt.Sprintf(
			"\nTest case %d: %s\n%v\n%v<--",
			idx, tc.Name, tc.Query, tc.Query[:tc.Column]))

		// Load the document.
		diagnostics, err := lsp_service.DidOpen(self.Ctx,
			&protocol.DidOpenTextDocumentParams{
				TextDocument: protocol.TextDocumentItem{
					URI:  doc,
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
					URI: doc,
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
	}

	goldie.Assert(self.T(), "TestCompletion",
		[]byte(strings.Join(golden, "\n")))
}

// A completion request may legitimately reference a position beyond
// the end of the document - e.g. when the client's view of the text
// is briefly ahead of the server, or a trigger fires on an empty
// line. The server must clamp the position instead of panicking (a
// panic here previously crashed the entire frontend).
func (self *LSPTestSuite) TestCompletionPositionPastEndOfDocument() {
	lsp_service := lsp.NewLSPServer(self.ConfigObj).(*lsp.LSPServer)

	for _, tc := range []struct {
		Name      string
		Line      uint32
		Character uint32
	}{{
		Name:      "character past end of line",
		Line:      0,
		Character: 101,
	}, {
		Name:      "line past end of document",
		Line:      50,
		Character: 10,
	}} {
		doc := uri.URI(fmt.Sprintf("file:///XXX-past-end-%v", tc.Name))

		_, err := lsp_service.DidOpen(self.Ctx,
			&protocol.DidOpenTextDocumentParams{
				TextDocument: protocol.TextDocumentItem{
					URI:  doc,
					Text: "SELECT * FROM parse.",
				},
			})
		assert.NoError(self.T(), err)

		req := &protocol.CompletionParams{
			TextDocumentPositionParams: protocol.TextDocumentPositionParams{
				TextDocument: protocol.TextDocumentIdentifier{
					URI: doc,
				},
				Position: protocol.Position{
					Line:      tc.Line,
					Character: tc.Character,
				},
			},
		}

		// Must not panic and must return a valid result.
		completions, err := lsp_service.Completion(self.Ctx, req)
		assert.NoError(self.T(), err)
		assert.NotNil(self.T(), completions)
	}
}
