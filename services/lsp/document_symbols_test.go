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

func (self *LSPTestSuite) TestDocumentSymbols() {
	lsp_service := lsp.NewLSPServer(self.ConfigObj).(*lsp.LSPServer)

	doc := uri.URI("file:///XXX")
	_, err := lsp_service.DidOpen(self.Ctx,
		&protocol.DidOpenTextDocumentParams{
			TextDocument: protocol.TextDocumentItem{
				URI: doc,
				Text: "LET MyVar = 5\n" +
					"LET OtherVar = SELECT * FROM pslist(pid=MyVar)\n" +
					"LET MyFunc(X, Y) = X + Y\n" +
					"SELECT OtherVar FROM scope()\n",
			},
		})
	assert.NoError(self.T(), err)

	symbols, err := lsp_service.DocumentSymbol(self.Ctx,
		&protocol.DocumentSymbolParams{
			TextDocument: protocol.TextDocumentIdentifier{
				URI: doc,
			},
		})
	assert.NoError(self.T(), err)

	var golden []string
	for _, symbol := range symbols.(protocol.DocumentSymbolSlice) {
		golden = append(golden, fmt.Sprintf(
			"Name: %v Kind: %v Range: %v-%v",
			symbol.Name, symbol.Kind,
			symbol.Range.Start, symbol.Range.End))
	}

	goldie.Assert(self.T(), "TestDocumentSymbols",
		[]byte(strings.Join(golden, "\n")))
}