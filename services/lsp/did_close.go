package lsp

import (
	"context"

	"go.lsp.dev/protocol"
)

// DidClose handles the textDocument/didClose notification. The
// document is removed from the cache and an empty diagnostic list is
// returned so the proxy can clear the diagnostics published for the
// closed document.
func (self *LSPServer) DidClose(
	ctx context.Context,
	params *protocol.DidCloseTextDocumentParams) ([]*protocol.Diagnostic, error) {

	self.mu.Lock()
	delete(self.documents, params.TextDocument.URI)
	self.mu.Unlock()

	return []*protocol.Diagnostic{}, nil
}
