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
	formattingTC = []struct {
		Name  string
		Query string
	}{{
		Name:  "Reformat a query with long lines",
		Query: "LET X = SELECT * FROM pslist(pid=1)\nSELECT X FROM scope()\n",
	}, {
		Name:  "Reformat a nested function",
		Query: "SELECT * FROM glob(globs=lowcase(string='*'))",
	}, {
		Name:  "Refuse unparseable query",
		Query: "SELECT * FROM",
	}}
)

func (self *LSPTestSuite) TestFormatting() {
	lsp_service := lsp.NewLSPServer(self.ConfigObj).(*lsp.LSPServer)

	var golden []string

	for idx, tc := range formattingTC {
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

		// Request the format.
		edits, err := lsp_service.Formatting(self.Ctx,
			&protocol.DocumentFormattingParams{
				TextDocument: protocol.TextDocumentIdentifier{
					URI: doc,
				},
			})
		assert.NoError(self.T(), err)

		golden = append(golden, "Formatting:")
		golden = append(golden, lsp.DumpProtool(edits))
	}

	goldie.Assert(self.T(), "TestFormatting",
		[]byte(strings.Join(golden, "\n")))
}
