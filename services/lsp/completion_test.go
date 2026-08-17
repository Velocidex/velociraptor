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
		Column: 11,
	}, {
		Name:   "Complete VQL plugin",
		Query:  "SELECT * FROM scope()",
		Column: 17,
	}, {
		Name:   "Complete VQL nested function",
		Query:  "SELECT * FROM glob(globs=lowcase(string='*'))",
		Column: 46 - 17,
	}, {
		Name:   "Complete plugin all args",
		Query:  "SELECT * FROM glob()",
		Column: 19,
	}, {
		Name:   "Complete a plugin args",
		Query:  "SELECT * FROM glob(gloXXX='*')",
		Column: 21,
	}, {
		Name:   "Complete some plugin args (accessor already exists, so should not be completed) ",
		Query:  "SELECT * FROM glob(gloXXX='*', accessor='file')",
		Column: 19,
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
