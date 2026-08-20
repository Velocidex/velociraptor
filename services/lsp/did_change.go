package lsp

import (
	"context"

	"go.lsp.dev/protocol"
	"www.velocidex.com/golang/velociraptor/utils"
)

// DidChange handles the textDocument/didChange notification. We
// advertise full sync so each change contains the whole document text.
//
// Typing trigger characters like '.' or '(' often makes the VQL
// temporarily invalid (e.g. "pslist(?" or "Artifact."). When the parse
// fails the analysis state is empty, so completion and other features
// have nothing to work with. To keep the IDE usable while typing we
// retain the analysis state from the last good parse.
func (self *LSPServer) DidChange(
	ctx context.Context,
	params *protocol.DidChangeTextDocumentParams) ([]*protocol.Diagnostic, error) {

	self.mu.Lock()
	old_doc, pres := self.documents[params.TextDocument.URI]
	self.mu.Unlock()
	if !pres {
		return nil, utils.NotFoundError
	}

	if len(params.ContentChanges) == 0 {
		return nil, utils.NotFoundError
	}

	// We advertise full sync so the client sends the whole document
	// in a TextDocumentContentChangeWholeDocument.
	text := ""
	if change, ok := params.ContentChanges[len(params.ContentChanges)-1].(*protocol.TextDocumentContentChangeWholeDocument); ok {
		text = change.Text
	}

	document, err := NewDocument(ctx, self.config_obj, params.TextDocument.URI, text)
	if err != nil {
		return nil, err
	}

	// If the new document failed to parse, the analysis state is
	// empty. Retain the old analysis state so completion and other
	// features can still work on the partially typed text.
	if len(document.AnalysisState.Callsites) == 0 &&
		len(document.AnalysisState.Definitions) == 0 &&
		len(document.Errors) > 0 {
		document.AnalysisState = old_doc.AnalysisState
	}

	self.mu.Lock()
	self.documents[params.TextDocument.URI] = document
	self.mu.Unlock()

	return document.Diagnostics(), nil
}
