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
	foldingTC = []struct {
		Name  string
		Query string
	}{{
		Name: "Fold a multi line LET query",
		Query: "LET X = SELECT *\n" +
			"FROM pslist(pid=1)\n" +
			"SELECT X FROM scope()\n",
	}, {
		Name:  "No fold for a single line",
		Query: "SELECT * FROM pslist(pid=1)",
	}, {
		Name: "Fold a multi line query",
		Query: "SELECT *\n" +
			"FROM pslist(pid=1)",
	}}
)

func (self *LSPTestSuite) TestFoldingRanges() {
	lsp_service := lsp.NewLSPServer(self.ConfigObj).(*lsp.LSPServer)

	var golden []string

	for idx, tc := range foldingTC {
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

		ranges, err := lsp_service.FoldingRanges(self.Ctx,
			&protocol.FoldingRangeParams{
				TextDocument: protocol.TextDocumentIdentifier{
					URI: doc,
				},
			})
		assert.NoError(self.T(), err)

		golden = append(golden, "Folding ranges:")
		golden = append(golden, lsp.DumpProtool(ranges))
	}

	goldie.Assert(self.T(), "TestFoldingRanges",
		[]byte(strings.Join(golden, "\n")))
}
