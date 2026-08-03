/*
   Velociraptor - Dig Deeper
   Copyright (C) 2019-2025 Rapid7 Inc.

   This program is free software: you can redistribute it and/or modify
   it under the terms of the GNU Affero General Public License as published
   by the Free Software Foundation, either version 3 of the License, or
   (at your option) any later version.

   This program is distributed in the hope that it will be useful,
   but WITHOUT ANY WARRANTY; without even the implied warranty of
   MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
   GNU Affero General Public License for more details.

   You should have received a copy of the GNU Affero General Public License
   along with this program.  If not, see <https://www.gnu.org/licenses/>.
*/

package lsp

import (
	"context"

	"go.lsp.dev/protocol"
	"go.lsp.dev/uri"
)

// DidOpen implements the textDocument/didOpen notification.
func (self *Server) DidOpen(
	ctx context.Context, params *protocol.DidOpenTextDocumentParams) error {

	self.mu.Lock()
	self.documents[params.TextDocument.URI] = params.TextDocument.Text
	self.mu.Unlock()

	return self.publishDiagnostics(ctx, params.TextDocument.URI)
}

// DidChange implements the textDocument/didChange notification.
//
// We advertise full sync so each change contains the whole document text;
// we simply replace our stored copy.
func (self *Server) DidChange(
	ctx context.Context, params *protocol.DidChangeTextDocumentParams) error {

	self.mu.Lock()
	if len(params.ContentChanges) > 0 {
		// We advertise full sync, so the client sends the whole
		// document in a TextDocumentContentChangeWholeDocument.
		if change, ok := params.ContentChanges[len(params.ContentChanges)-1].(
			*protocol.TextDocumentContentChangeWholeDocument); ok {
			self.documents[params.TextDocument.URI] = change.Text
		}
	}
	self.mu.Unlock()

	return self.publishDiagnostics(ctx, params.TextDocument.URI)
}

// DidClose implements the textDocument/didClose notification.
func (self *Server) DidClose(
	ctx context.Context, params *protocol.DidCloseTextDocumentParams) error {

	self.mu.Lock()
	delete(self.documents, params.TextDocument.URI)
	self.mu.Unlock()

	// Clear any diagnostics for the closed document.
	return self.publishDiagnostics(ctx, params.TextDocument.URI)
}

// DidSave implements the textDocument/didSave notification.
func (self *Server) DidSave(
	ctx context.Context, params *protocol.DidSaveTextDocumentParams) error {

	return self.publishDiagnostics(ctx, params.TextDocument.URI)
}

// publishDiagnostics parses the document and pushes diagnostics to the
// client.
func (self *Server) publishDiagnostics(ctx context.Context, doc_uri uri.URI) error {
	self.mu.Lock()
	document, pres := self.documents[doc_uri]
	client := self.client
	self.mu.Unlock()

	diagnostics := []protocol.Diagnostic{}
	if pres {
		diagnostics = self.registry.Validate(document)
	}

	client.PublishDiagnostics(ctx, &protocol.PublishDiagnosticsParams{
		URI:         doc_uri,
		Diagnostics: diagnostics,
	})
	return nil
}
