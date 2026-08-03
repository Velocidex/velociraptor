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
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.lsp.dev/protocol"
	"go.lsp.dev/uri"
)

// mockClient records diagnostics pushed via textDocument/publishDiagnostics.
type mockClient struct {
	protocol.UnimplementedClient

	mu         sync.Mutex
	published  map[uri.URI][]protocol.Diagnostic
	clearCalls int
}

func newMockClient() *mockClient {
	return &mockClient{published: make(map[uri.URI][]protocol.Diagnostic)}
}

func (self *mockClient) PublishDiagnostics(
	ctx context.Context, params *protocol.PublishDiagnosticsParams) error {

	self.mu.Lock()
	defer self.mu.Unlock()
	self.published[params.URI] = params.Diagnostics
	if len(params.Diagnostics) == 0 {
		self.clearCalls++
	}
	return nil
}

func (self *mockClient) get(uri_ uri.URI) []protocol.Diagnostic {
	self.mu.Lock()
	defer self.mu.Unlock()
	return self.published[uri_]
}

func newTestServer() (*Server, *mockClient) {
	registry := newTestRegistry()
	server := NewServer(registry)
	client := newMockClient()
	server.client = client
	return server, client
}

func TestServerInitializeCapabilities(t *testing.T) {
	server, _ := newTestServer()

	// Provide a client via the context, as NewServer wiring does.
	ctx := protocol.WithClient(context.Background(), newMockClient())
	result, err := server.Initialize(ctx, &protocol.InitializeParams{})
	require.NoError(t, err)

	assert.Equal(t, protocol.TextDocumentSyncKindFull,
		result.Capabilities.TextDocumentSync)
	assert.NotNil(t, result.Capabilities.DiagnosticProvider)
	opts, ok := result.Capabilities.DiagnosticProvider.(*protocol.DiagnosticOptions)
	require.True(t, ok, "DiagnosticProvider should be *DiagnosticOptions")
	assert.Equal(t, "vql", *opts.Identifier)
	assert.Equal(t, "vql-lsp", result.ServerInfo.Name)
}

func TestServerDidOpenAndPullDiagnostic(t *testing.T) {
	server, client := newTestServer()
	ctx := context.Background()

	doc_uri := uri.MustParse("file:///tmp/bad.vql")
	err := server.DidOpen(ctx, &protocol.DidOpenTextDocumentParams{
		TextDocument: protocol.TextDocumentItem{
			URI:     doc_uri,
			Text:    "SELECT * FROM pslist(foo=1)",
			Version: 1,
		},
	})
	require.NoError(t, err)

	// Pull-based request should return the diagnostic.
	report, err := server.Diagnostic(ctx, &protocol.DocumentDiagnosticParams{
		TextDocument: protocol.TextDocumentIdentifier{URI: doc_uri},
	})
	require.NoError(t, err)

	items := reportItems(t, report)
	require.Len(t, items, 1)
	assert.Equal(t, "Unknown argument 'foo' for plugin 'pslist'",
		messageOf(items[0]))

	// Push-based should also have fired on didOpen.
	pushed := client.get(doc_uri)
	require.Len(t, pushed, 1)
	assert.Equal(t, "Unknown argument 'foo' for plugin 'pslist'",
		messageOf(pushed[0]))
}

func TestServerDidChangeUpdatesDocument(t *testing.T) {
	server, _ := newTestServer()
	ctx := context.Background()

	doc_uri := uri.MustParse("file:///tmp/change.vql")
	require.NoError(t, server.DidOpen(ctx, &protocol.DidOpenTextDocumentParams{
		TextDocument: protocol.TextDocumentItem{
			URI:     doc_uri,
			Text:    "SELECT * FROM pslist()",
			Version: 1,
		},
	}))

	// Change to a document with an error - full sync.
	require.NoError(t, server.DidChange(ctx, &protocol.DidChangeTextDocumentParams{
		TextDocument: protocol.VersionedTextDocumentIdentifier{
			TextDocumentIdentifier: protocol.TextDocumentIdentifier{URI: doc_uri},
			Version:                2,
		},
		ContentChanges: []protocol.TextDocumentContentChangeEvent{
			&protocol.TextDocumentContentChangeWholeDocument{
				Text: "SELECT * FROM pslist(badarg=1)",
			},
		},
	}))

	report, err := server.Diagnostic(ctx, &protocol.DocumentDiagnosticParams{
		TextDocument: protocol.TextDocumentIdentifier{URI: doc_uri},
	})
	require.NoError(t, err)
	items := reportItems(t, report)
	require.Len(t, items, 1)
	assert.Equal(t, "Unknown argument 'badarg' for plugin 'pslist'",
		messageOf(items[0]))
}

func TestServerDidCloseClearsDiagnostics(t *testing.T) {
	server, client := newTestServer()
	ctx := context.Background()

	doc_uri := uri.MustParse("file:///tmp/close.vql")
	require.NoError(t, server.DidOpen(ctx, &protocol.DidOpenTextDocumentParams{
		TextDocument: protocol.TextDocumentItem{
			URI:     doc_uri,
			Text:    "SELECT * FROM pslist(foo=1)",
			Version: 1,
		},
	}))
	require.Len(t, client.get(doc_uri), 1)

	require.NoError(t, server.DidClose(ctx, &protocol.DidCloseTextDocumentParams{
		TextDocument: protocol.TextDocumentIdentifier{URI: doc_uri},
	}))

	// Pull on a closed document returns no items.
	report, err := server.Diagnostic(ctx, &protocol.DocumentDiagnosticParams{
		TextDocument: protocol.TextDocumentIdentifier{URI: doc_uri},
	})
	require.NoError(t, err)
	require.Empty(t, reportItems(t, report))

	// Push should have cleared them.
	require.Empty(t, client.get(doc_uri))
}

func TestServerPullUnknownDocument(t *testing.T) {
	server, _ := newTestServer()
	ctx := context.Background()

	// Never opened this document.
	report, err := server.Diagnostic(ctx, &protocol.DocumentDiagnosticParams{
		TextDocument: protocol.TextDocumentIdentifier{
			URI: uri.MustParse("file:///tmp/never-opened.vql"),
		},
	})
	require.NoError(t, err)
	require.Empty(t, reportItems(t, report))
}

func TestServerShutdownClosesDone(t *testing.T) {
	server, _ := newTestServer()
	require.NoError(t, server.Shutdown(context.Background()))
	select {
	case <-server.Done():
	default:
		t.Fatalf("Done() channel should be closed after Shutdown")
	}
}

// reportItems extracts the diagnostic items from a pull-based report.
func reportItems(t *testing.T, report protocol.DocumentDiagnosticReport) []protocol.Diagnostic {
	t.Helper()
	switch r := report.(type) {
	case *protocol.RelatedFullDocumentDiagnosticReport:
		return r.FullDocumentDiagnosticReport.Items
	case *protocol.RelatedUnchangedDocumentDiagnosticReport:
		return nil
	default:
		t.Fatalf("unexpected report type %T", report)
		return nil
	}
}
