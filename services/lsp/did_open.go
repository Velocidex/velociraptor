package lsp

import (
	"context"

	"go.lsp.dev/protocol"
)

func (self *LSPServer) DidOpen(
	ctx context.Context,
	params *protocol.DidOpenTextDocumentParams) (
	[]*protocol.Diagnostic, error) {

	document, err := NewDocument(
		ctx, self.config_obj, params.TextDocument.URI,
		params.TextDocument.Text)
	if err != nil {
		return nil, err
	}

	if document.AnalysisState.FailedToParse {
		existing, err := self.GetDoc(params.TextDocument.URI)
		if err == nil {
			existing.UpdateTextFromDocument(document)
			document = existing
		}
	}

	self.setDoc(params.TextDocument.URI, document)

	return document.Diagnostics(), nil
}
