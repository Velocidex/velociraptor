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

func (self *LSPTestSuite) TestWorkspaceSymbols() {
	lsp_service := lsp.NewLSPServer(self.ConfigObj).(*lsp.LSPServer)

	doc := uri.URI("file:///XXX")
	_, err := lsp_service.DidOpen(self.Ctx,
		&protocol.DidOpenTextDocumentParams{
			TextDocument: protocol.TextDocumentItem{
				URI: doc,
				Text: "LET MyVar = 5\n" +
					"LET OtherVar = SELECT * FROM pslist(pid=MyVar)\n" +
					"SELECT OtherVar FROM scope()\n",
			},
		})
	assert.NoError(self.T(), err)

	var golden []string

	for _, query := range []string{"", "Var", "pslist", "XXXXXX"} {
		symbols, err := lsp_service.Symbols(self.Ctx,
			&protocol.WorkspaceSymbolParams{
				Query: query,
			})
		assert.NoError(self.T(), err)

		golden = append(golden, fmt.Sprintf(
			"\nWorkspace symbols for query: %v\n%v",
			query, lsp.DumpProtool(symbols)))
	}

	goldie.Assert(self.T(), "TestWorkspaceSymbols",
		[]byte(strings.Join(golden, "\n")))
}
