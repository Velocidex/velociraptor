package lsp_test

import (
	"go.lsp.dev/protocol"
	"go.lsp.dev/uri"
	"www.velocidex.com/golang/velociraptor/services/lsp"
	"www.velocidex.com/golang/velociraptor/vtesting/assert"
)

func (self *LSPTestSuite) TestDidClose() {
	lsp_service := lsp.NewLSPServer(self.ConfigObj).(*lsp.LSPServer)

	doc := uri.URI("file:///XXX")

	// Open a valid document.
	_, err := lsp_service.DidOpen(self.Ctx,
		&protocol.DidOpenTextDocumentParams{
			TextDocument: protocol.TextDocumentItem{
				URI:  doc,
				Text: "SELECT * FROM glob(globs='*')",
			},
		})
	assert.NoError(self.T(), err)

	// The document is now cached - pull diagnostics works.
	_, err = lsp_service.Diagnostic(self.Ctx,
		&protocol.DocumentDiagnosticParams{
			TextDocument: protocol.TextDocumentIdentifier{
				URI: doc,
			},
		})
	assert.NoError(self.T(), err)

	// Close it - the document should be removed and diagnostics
	// cleared.
	diags, err := lsp_service.DidClose(self.Ctx,
		&protocol.DidCloseTextDocumentParams{
			TextDocument: protocol.TextDocumentIdentifier{
				URI: doc,
			},
		})
	assert.NoError(self.T(), err)
	assert.Equal(self.T(), 0, len(diags))

	// The document is no longer cached - pull diagnostics now
	// fails with NotFoundError.
	_, err = lsp_service.Diagnostic(self.Ctx,
		&protocol.DocumentDiagnosticParams{
			TextDocument: protocol.TextDocumentIdentifier{
				URI: doc,
			},
		})
	assert.Error(self.T(), err)
}
