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
	inlayHintTC = []struct {
		Name  string
		Query string
	}{{
		Name:  "Types of the named arguments",
		Query: "SELECT * FROM glob(globs='/foo', accessor='file')",
	}, {
		Name:  "Types of a function call",
		Query: "SELECT upcase(string='abc') FROM scope()",
	}, {
		Name:  "No hints for non built in calls",
		Query: "SELECT * FROM MyUnknownPlugin(foo=1)",
	}}
)

func (self *LSPTestSuite) TestInlayHints() {
	lsp_service := lsp.NewLSPServer(self.ConfigObj).(*lsp.LSPServer)

	var golden []string

	for idx, tc := range inlayHintTC {
		doc := uri.URI(fmt.Sprintf("file:///XXX%d", idx))

		golden = append(golden, fmt.Sprintf(
			"\nTest case %d: %s\n%v",
			idx, tc.Name, tc.Query))

		// Load the document.
		_, err := lsp_service.DidOpen(self.Ctx,
			&protocol.DidOpenTextDocumentParams{
				TextDocument: protocol.TextDocumentItem{
					URI:  doc,
					Text: tc.Query,
				},
			})
		assert.NoError(self.T(), err)

		hints, err := lsp_service.InlayHint(self.Ctx,
			&protocol.InlayHintParams{
				TextDocument: protocol.TextDocumentIdentifier{
					URI: doc,
				},
			})
		assert.NoError(self.T(), err)

		golden = append(golden, "Inlay hints:")
		golden = append(golden, lsp.DumpProtool(hints))
	}

	goldie.Assert(self.T(), "TestInlayHints",
		[]byte(strings.Join(golden, "\n")))
}
