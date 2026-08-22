package lsp_test

import (
	"strings"

	"go.lsp.dev/protocol"
	"go.lsp.dev/uri"
	"www.velocidex.com/golang/velociraptor/services/lsp"
	"www.velocidex.com/golang/velociraptor/vtesting/assert"
	"www.velocidex.com/golang/velociraptor/vtesting/goldie"
)

func (self *LSPTestSuite) TestCodeActionRemoveUnknownArg() {
	lsp_service := lsp.NewLSPServer(self.ConfigObj).(*lsp.LSPServer)

	doc := uri.URI("file:///XXX")
	text := "SELECT * FROM pslist(foo=1)"

	// The diagnostics are only exposed to the client via didOpen
	// which publishes them over the wire. For the unit test we build
	// a diagnostic matching the unknown argument error.
	diags, err := lsp_service.DidOpen(self.Ctx,
		&protocol.DidOpenTextDocumentParams{
			TextDocument: protocol.TextDocumentItem{
				URI:  doc,
				Text: text,
			},
		})
	assert.NoError(self.T(), err)
	assert.Equal(self.T(), 1, len(diags))

	var diagnostics []protocol.Diagnostic
	for _, d := range diags {
		diagnostics = append(diagnostics, *d)
	}

	actions, err := lsp_service.CodeAction(self.Ctx,
		&protocol.CodeActionParams{
			TextDocument: protocol.TextDocumentIdentifier{
				URI: doc,
			},
			Range: protocol.Range{
				Start: protocol.Position{Line: 0, Character: 0},
				End:   protocol.Position{Line: 0, Character: 50},
			},
			Context: protocol.CodeActionContext{
				Diagnostics: diagnostics,
			},
		})
	assert.NoError(self.T(), err)

	golden := []string{"Code actions:"}
	for _, action := range actions {
		golden = append(golden, lsp.DumpProtool(action))
	}

	goldie.Assert(self.T(), "TestCodeActionRemoveUnknownArg",
		[]byte(strings.Join(golden, "\n")))
}

func (self *LSPTestSuite) TestCodeActionFormatDocument() {
	lsp_service := lsp.NewLSPServer(self.ConfigObj).(*lsp.LSPServer)

	doc := uri.URI("file:///XXX")
	text := "SELECT * FROM pslist(pid=1)"
	_, err := lsp_service.DidOpen(self.Ctx,
		&protocol.DidOpenTextDocumentParams{
			TextDocument: protocol.TextDocumentItem{
				URI:  doc,
				Text: text,
			},
		})
	assert.NoError(self.T(), err)

	actions, err := lsp_service.CodeAction(self.Ctx,
		&protocol.CodeActionParams{
			TextDocument: protocol.TextDocumentIdentifier{
				URI: doc,
			},
			Range: protocol.Range{
				Start: protocol.Position{Line: 0, Character: 0},
				End:   protocol.Position{Line: 0, Character: 50},
			},
			Context: protocol.CodeActionContext{
				Only: []protocol.CodeActionKind{
					protocol.CodeActionKindSource,
				},
			},
		})
	assert.NoError(self.T(), err)

	var golden []string
	for _, action := range actions {
		golden = append(golden, lsp.DumpProtool(action))
	}

	goldie.Assert(self.T(), "TestCodeActionFormatDocument",
		[]byte(strings.Join(golden, "\n")))
}

// A document with syntax errors can not be formatted. The source
// format action must be omitted from the response rather than
// appended as a null entry - a null in the actions array crashes
// editor clients when they convert the result.
func (self *LSPTestSuite) TestCodeActionNoNullEntries() {
	lsp_service := lsp.NewLSPServer(self.ConfigObj).(*lsp.LSPServer)

	doc := uri.URI("file:///XXX-broken")
	text := "SELECT * FROM pars.\nLET MyFunc(X, Y) = X + Y\n"

	_, err := lsp_service.DidOpen(self.Ctx,
		&protocol.DidOpenTextDocumentParams{
			TextDocument: protocol.TextDocumentItem{
				URI:  doc,
				Text: text,
			},
		})
	assert.NoError(self.T(), err)

	actions, err := lsp_service.CodeAction(self.Ctx,
		&protocol.CodeActionParams{
			TextDocument: protocol.TextDocumentIdentifier{
				URI: doc,
			},
			Range: protocol.Range{
				Start: protocol.Position{Line: 0, Character: 0},
				End:   protocol.Position{Line: 0, Character: 1},
			},
			Context: protocol.CodeActionContext{},
		})
	assert.NoError(self.T(), err)

	for i, action := range actions {
		assert.NotNil(self.T(), action, "action %d is nil", i)
	}
}
