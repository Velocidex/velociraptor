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

func (self *LSPTestSuite) TestReferences() {
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

	// Reference from the definition with the declaration included.
	refs, err := lsp_service.References(self.Ctx,
		&protocol.ReferenceParams{
			TextDocumentPositionParams: protocol.TextDocumentPositionParams{
				TextDocument: protocol.TextDocumentIdentifier{
					URI: doc,
				},
				Position: protocol.Position{Line: 0, Character: 4},
			},
			Context: protocol.ReferenceContext{
				IncludeDeclaration: true,
			},
		})
	assert.NoError(self.T(), err)

	var golden []string
	golden = append(golden, "References for X from definition (include declaration):")
	for _, ref := range refs {
		golden = append(golden, lsp.DumpProtool(ref))
	}

	// Reference from a use without the declaration.
	refs, err = lsp_service.References(self.Ctx,
		&protocol.ReferenceParams{
			TextDocumentPositionParams: protocol.TextDocumentPositionParams{
				TextDocument: protocol.TextDocumentIdentifier{
					URI: doc,
				},
				Position: protocol.Position{Line: 1, Character: 7},
			},
			Context: protocol.ReferenceContext{
				IncludeDeclaration: false,
			},
		})
	assert.NoError(self.T(), err)

	golden = append(golden, "References for X from a use (no declaration):")
	for _, ref := range refs {
		golden = append(golden, lsp.DumpProtool(ref))
	}

	// No references for unknown identifiers.
	refs, err = lsp_service.References(self.Ctx,
		&protocol.ReferenceParams{
			TextDocumentPositionParams: protocol.TextDocumentPositionParams{
				TextDocument: protocol.TextDocumentIdentifier{
					URI: doc,
				},
				Position: protocol.Position{Line: 0, Character: 14},
			},
			Context: protocol.ReferenceContext{
				IncludeDeclaration: true,
			},
		})
	assert.NoError(self.T(), err)
	golden = append(golden, fmt.Sprintf(
		"References for a plugin name: %v", refs))

	goldie.Assert(self.T(), "TestReferences",
		[]byte(strings.Join(golden, "\n")))
}
