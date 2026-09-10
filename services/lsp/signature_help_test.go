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
	signatureHelpTC = []struct {
		Name   string
		Query  string
		Column uint32
	}{{
		Name:   "Signature of a plugin",
		Query:  "SELECT * FROM pslist(pid=1)",
		Column: 22,
	}, {
		Name:   "Signature of a function",
		Query:  "SELECT upcase(string='x') FROM scope()",
		Column: 17,
	}, {
		Name:   "Signature of a nested function",
		Query:  "SELECT * FROM glob(globs=lowcase(string='*'))",
		Column: 37,
	}, {
		Name:   "No signature outside a call",
		Query:  "SELECT * FROM pslist(pid=1)",
		Column: 10,
	}}
)

func (self *LSPTestSuite) TestSignatureHelp() {
	lsp_service := lsp.NewLSPServer(self.ConfigObj).(*lsp.LSPServer)

	var golden []string

	for idx, tc := range signatureHelpTC {
		doc := uri.URI(fmt.Sprintf("file:///XXX%d", idx))

		golden = append(golden, fmt.Sprintf(
			"\nTest case %d: %s\n%v\n%v<--",
			idx, tc.Name, tc.Query, tc.Query[:tc.Column]))

		// Load the document.
		_, err := lsp_service.DidOpen(self.Ctx,
			&protocol.DidOpenTextDocumentParams{
				TextDocument: protocol.TextDocumentItem{
					URI:  doc,
					Text: tc.Query,
				},
			})
		assert.NoError(self.T(), err)

		// Request signature help at the point.
		sig, err := lsp_service.SignatureHelp(self.Ctx,
			&protocol.SignatureHelpParams{
				TextDocumentPositionParams: protocol.TextDocumentPositionParams{
					TextDocument: protocol.TextDocumentIdentifier{
						URI: doc,
					},
					Position: protocol.Position{
						Line:      0,
						Character: tc.Column,
					},
				},
			})
		assert.NoError(self.T(), err)

		golden = append(golden, "SignatureHelp:")
		golden = append(golden, lsp.DumpProtool(sig))
	}

	goldie.Assert(self.T(), "TestSignatureHelp",
		[]byte(strings.Join(golden, "\n")))
}
