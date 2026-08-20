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

	self.setDoc(params.TextDocument.URI, document)

	return document.Diagnostics(), nil
}

func (self *LSPServer) DidChange(
	ctx context.Context, params *protocol.DidChangeTextDocumentParams) (
	[]*protocol.Diagnostic, error) {

	for _, event := range params.ContentChanges {
		// Only support full refresh for now.
		full_event, ok := event.(*protocol.TextDocumentContentChangeWholeDocument)
		if ok {
			document, err := NewDocument(
				ctx, self.config_obj, params.TextDocument.URI,
				full_event.Text)
			if err != nil {
				return nil, err
			}

			if document.AnalysisState.FailedToParse {
				existing, err := self.getDoc(params.TextDocument.URI)
				if err == nil {
					existing.UpdateTextFromDocument(document)
					document = existing
				}
			}

			self.setDoc(params.TextDocument.URI, document)
			return document.Diagnostics(), nil
		}
	}
	return nil, nil
}
