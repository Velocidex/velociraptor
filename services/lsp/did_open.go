package lsp

import (
	"context"

	"go.lsp.dev/protocol"
)

func (self *LSPServer) DidOpen(
	ctx context.Context,
	params *protocol.DidOpenTextDocumentParams) ([]*protocol.Diagnostic, error) {

	document, err := NewDocument(
		ctx, self.config_obj, params.TextDocument.URI,
		params.TextDocument.Text)
	if err != nil {
		return nil, err
	}

	self.mu.Lock()
	self.documents[params.TextDocument.URI] = document
	self.mu.Unlock()

	return document.Diagnostics(), nil
}
