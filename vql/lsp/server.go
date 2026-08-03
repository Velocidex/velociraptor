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

// Package lsp implements a Language Server for VQL.
//
// The server runs inside the Velociraptor binary (command `velociraptor
// lsp`) so that it has access to the full plugin and function registry.
// Documents are treated as single VQL query strings (virtual documents),
// which is the primary use case: an AI agent drafts a query, opens it as a
// virtual document, receives diagnostics back, fixes the query, and finally
// submits it to the Velociraptor API.
package lsp

import (
	"context"
	"sync"

	"go.lsp.dev/protocol"
	"go.lsp.dev/uri"
)

// Server is the LSP server for VQL. It embeds UnimplementedServer so that
// only the methods we care about need to be overridden.
type Server struct {
	protocol.UnimplementedServer

	mu sync.Mutex
	// documents maps a document URI to its current text content.
	documents map[uri.URI]string
	// registry holds the plugin/function introspection data.
	registry *Registry

	// client is used to push diagnostics to the client.
	client protocol.Client

	// done is closed when the client asks us to shut down or exit.
	done chan struct{}
	once sync.Once
}

func NewServer(registry *Registry) *Server {
	return &Server{
		documents: make(map[uri.URI]string),
		registry:  registry,
		done:      make(chan struct{}),
	}
}

// Done is closed when the client has asked the server to shut down or exit.
func (self *Server) Done() <-chan struct{} {
	return self.done
}

// Initialize implements the LSP initialize request.
func (self *Server) Initialize(
	ctx context.Context, params *protocol.InitializeParams) (*protocol.InitializeResult, error) {

	self.mu.Lock()
	self.client, _ = protocol.ClientFromContext(ctx)
	self.mu.Unlock()

	return &protocol.InitializeResult{
		Capabilities: protocol.ServerCapabilities{
			// Sync whole documents on open/change.
			TextDocumentSync: protocol.TextDocumentSyncKindFull,

			// The server pushes diagnostics via
			// textDocument/publishDiagnostics.
			DiagnosticProvider: &protocol.DiagnosticOptions{
				InterFileDependencies: false,
				WorkspaceDiagnostics:  false,
				Identifier:            ptr("vql"),
			},

			// The server provides a document outline.
			DocumentSymbolProvider: protocol.Boolean(true),

			// The server provides hover documentation.
			HoverProvider: protocol.Boolean(true),

			// The server provides autocompletion. Typing '.' (as in
			// Artifact.Linux.Sys) or '(' (as in pslist() ) should
			// trigger completion.
			CompletionProvider: &protocol.CompletionOptions{
				TriggerCharacters: []string{".", "("},
			},
		},
		ServerInfo: protocol.ServerInfo{
			Name:    "vql-lsp",
			Version: protocol.NewOptional("0.1"),
		},
	}, nil
}

// Initialized is a no-op.
func (self *Server) Initialized(ctx context.Context, params *protocol.InitializedParams) error {
	return nil
}

// Diagnostic implements the pull-based textDocument/diagnostic request
// (LSP 3.17). Some clients, including opencode, use pull-based diagnostics
// instead of relying on push-based textDocument/publishDiagnostics.
func (self *Server) Diagnostic(
	ctx context.Context, params *protocol.DocumentDiagnosticParams) (protocol.DocumentDiagnosticReport, error) {

	self.mu.Lock()
	text, pres := self.documents[params.TextDocument.URI]
	self.mu.Unlock()

	if !pres {
		return &protocol.RelatedFullDocumentDiagnosticReport{
			FullDocumentDiagnosticReport: protocol.FullDocumentDiagnosticReport{
				Kind:  "full",
				Items: []protocol.Diagnostic{},
			},
		}, nil
	}

	items := self.registry.Validate(text)

	return &protocol.RelatedFullDocumentDiagnosticReport{
		FullDocumentDiagnosticReport: protocol.FullDocumentDiagnosticReport{
			Kind:  "full",
			Items: items,
		},
	}, nil
}

// Shutdown tells the client the server is shutting down and that it will
// not be accepting new requests.
func (self *Server) Shutdown(ctx context.Context) error {
	self.once.Do(func() {
		close(self.done)
	})
	return nil
}

// Exit asks the server process to exit. After Shutdown this should be the
// last notification from the client.
func (self *Server) Exit(ctx context.Context) error {
	self.once.Do(func() {
		close(self.done)
	})
	return nil
}

func ptr[T any](value T) *T {
	return &value
}
