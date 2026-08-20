package lsp_test

import (
	"strings"

	"go.lsp.dev/protocol"
	"go.lsp.dev/uri"
	"www.velocidex.com/golang/velociraptor/services/lsp"
	"www.velocidex.com/golang/velociraptor/vtesting/assert"
	"www.velocidex.com/golang/velociraptor/vtesting/goldie"
)

func (self *LSPTestSuite) TestDidChange() {
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

	var golden []string

	// Change to a valid document - the analysis should be updated.
	diags, err := lsp_service.DidChange(self.Ctx,
		&protocol.DidChangeTextDocumentParams{
			TextDocument: protocol.VersionedTextDocumentIdentifier{
				TextDocumentIdentifier: protocol.TextDocumentIdentifier{
					URI: doc,
				},
				Version: 2,
			},
			ContentChanges: []protocol.TextDocumentContentChangeEvent{
				&protocol.TextDocumentContentChangeWholeDocument{
					Text: "SELECT * FROM glob(globs='*', accessor='file')",
				},
			},
		})
	assert.NoError(self.T(), err)
	golden = append(golden, "Diagnostics after valid change:")
	golden = append(golden, lsp.DumpProtool(diags))

	// Now break the parse by typing a trigger character. The old
	// analysis should be retained so completion still works.
	diags, err = lsp_service.DidChange(self.Ctx,
		&protocol.DidChangeTextDocumentParams{
			TextDocument: protocol.VersionedTextDocumentIdentifier{
				TextDocumentIdentifier: protocol.TextDocumentIdentifier{
					URI: doc,
				},
				Version: 3,
			},
			ContentChanges: []protocol.TextDocumentContentChangeEvent{
				&protocol.TextDocumentContentChangeWholeDocument{
					Text: "SELECT * FROM glob(globs='*'?",
				},
			},
		})
	assert.NoError(self.T(), err)
	golden = append(golden, "Diagnostics after broken change:")
	golden = append(golden, lsp.DumpProtool(diags))

	// Completion should still work from the retained analysis - the
	// callsite for glob() is still known even though the parse broke.
	completions, err := lsp_service.Completion(self.Ctx,
		&protocol.CompletionParams{
			TextDocumentPositionParams: protocol.TextDocumentPositionParams{
				TextDocument: protocol.TextDocumentIdentifier{
					URI: doc,
				},
				Position: protocol.Position{Line: 0, Character: 22},
			},
		})
	assert.NoError(self.T(), err)
	golden = append(golden, "Completions after broken change:")
	golden = append(golden, lsp.DumpProtool(completions))

	goldie.Assert(self.T(), "TestDidChange",
		[]byte(strings.Join(golden, "\n")))
}
