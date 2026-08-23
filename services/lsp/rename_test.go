package lsp_test

import (
	"strings"

	"go.lsp.dev/protocol"
	"go.lsp.dev/uri"
	"www.velocidex.com/golang/velociraptor/services/lsp"
	"www.velocidex.com/golang/velociraptor/vtesting/assert"
	"www.velocidex.com/golang/velociraptor/vtesting/goldie"
)

func (self *LSPTestSuite) TestRename() {
	lsp_service := lsp.NewLSPServer(self.ConfigObj).(*lsp.LSPServer)

	doc := uri.URI("file:///XXX")
	text := "LET X = SELECT * FROM pslist()\nSELECT X FROM scope()\nSELECT upcase(string=X) FROM scope()"
	_, err := lsp_service.DidOpen(self.Ctx,
		&protocol.DidOpenTextDocumentParams{
			TextDocument: protocol.TextDocumentItem{
				URI:  doc,
				Text: text,
			},
		})
	assert.NoError(self.T(), err)

	// Prepare rename on the definition.
	prepare, err := lsp_service.PrepareRename(self.Ctx,
		&protocol.PrepareRenameParams{
			TextDocumentPositionParams: protocol.TextDocumentPositionParams{
				TextDocument: protocol.TextDocumentIdentifier{
					URI: doc,
				},
				Position: protocol.Position{Line: 0, Character: 4},
			},
		})
	assert.NoError(self.T(), err)

	var golden []string
	golden = append(golden, "PrepareRename for LET X:")
	golden = append(golden, lsp.DumpProtool(prepare))

	// Prepare rename on a plugin name is rejected.
	prepare, err = lsp_service.PrepareRename(self.Ctx,
		&protocol.PrepareRenameParams{
			TextDocumentPositionParams: protocol.TextDocumentPositionParams{
				TextDocument: protocol.TextDocumentIdentifier{
					URI: doc,
				},
				Position: protocol.Position{Line: 0, Character: 21},
			},
		})
	assert.NoError(self.T(), err)
	golden = append(golden, "PrepareRename for a plugin name:")
	golden = append(golden, lsp.DumpProtool(prepare))

	// Rename the variable from a use.
	edit, err := lsp_service.Rename(self.Ctx,
		&protocol.RenameParams{
			TextDocumentPositionParams: protocol.TextDocumentPositionParams{
				TextDocument: protocol.TextDocumentIdentifier{
					URI: doc,
				},
				Position: protocol.Position{Line: 1, Character: 7},
			},
			NewName: "Y",
		})
	assert.NoError(self.T(), err)

	golden = append(golden, "Rename X to Y from a use:")
	golden = append(golden, lsp.DumpProtool(edit))

	goldie.Assert(self.T(), "TestRename",
		[]byte(strings.Join(golden, "\n")))
}
