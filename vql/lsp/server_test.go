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

	// Document symbol support should be advertised.
	assert.NotNil(t, result.Capabilities.DocumentSymbolProvider)
	_, ok = result.Capabilities.DocumentSymbolProvider.(protocol.Boolean)
	require.True(t, ok, "DocumentSymbolProvider should be protocol.Boolean")

	// Hover support should be advertised.
	assert.NotNil(t, result.Capabilities.HoverProvider)
	_, ok = result.Capabilities.HoverProvider.(protocol.Boolean)
	require.True(t, ok, "HoverProvider should be protocol.Boolean")

	// Completion support should be advertised with trigger characters.
	completion_opts := result.Capabilities.CompletionProvider
	require.NotNil(t, completion_opts)
	assert.Contains(t, completion_opts.TriggerCharacters, ".")
	assert.Contains(t, completion_opts.TriggerCharacters, "(")
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

// symbolsFromResult extracts the hierarchical symbol list from a
// documentSymbol result (a union).
func symbolsFromResult(t *testing.T, result protocol.DocumentSymbolResult) []protocol.DocumentSymbol {
	t.Helper()
	switch r := result.(type) {
	case protocol.DocumentSymbolSlice:
		return []protocol.DocumentSymbol(r)
	case protocol.SymbolInformationSlice:
		t.Fatalf("expected hierarchical symbols, got flat SymbolInformation")
		return nil
	default:
		t.Fatalf("unexpected DocumentSymbolResult type %T", result)
		return nil
	}
}

// findSymbol returns the first symbol with the given name at the top
// level of the tree.
func findSymbol(t *testing.T, symbols []protocol.DocumentSymbol, name string) *protocol.DocumentSymbol {
	t.Helper()
	for i := range symbols {
		if symbols[i].Name == name {
			return &symbols[i]
		}
	}
	t.Fatalf("symbol %q not found", name)
	return nil
}

func TestServerDocumentSymbols(t *testing.T) {
	server, _ := newTestServer()
	ctx := context.Background()

	doc_uri := uri.MustParse("file:///tmp/symbols.vql")
	err := server.DidOpen(ctx, &protocol.DidOpenTextDocumentParams{
		TextDocument: protocol.TextDocumentItem{
			URI: doc_uri,
			Text: "LET Y = SELECT Foo FROM pslist(pid=1)\n" +
				"SELECT upcase(str=X), Bar AS baz FROM Artifact.Linux.Sys.Users() WHERE Foo > 3",
			Version: 1,
		},
	})
	require.NoError(t, err)

	result, err := server.DocumentSymbol(ctx, &protocol.DocumentSymbolParams{
		TextDocument: protocol.TextDocumentIdentifier{URI: doc_uri},
	})
	require.NoError(t, err)

	symbols := symbolsFromResult(t, result)
	require.Len(t, symbols, 2)

	// Statement 1: LET Y = SELECT ... - a variable wrapping a query.
	let := findSymbol(t, symbols, "Y")
	assert.Equal(t, protocol.SymbolKindVariable, let.Kind)
	require.Len(t, let.Children, 1)
	assert.Equal(t, "pslist", let.Children[0].Name)
	assert.Equal(t, protocol.SymbolKindFunction, let.Children[0].Kind)

	// Statement 2: the main query.
	query := findSymbol(t, symbols, "Artifact.Linux.Sys.Users")
	assert.Equal(t, protocol.SymbolKindFunction, query.Kind)
	require.Len(t, query.Children, 2)

	// Column 0: unaliased upcase(...) - name from source text, child
	// function upcase.
	col0 := query.Children[0]
	assert.Equal(t, protocol.SymbolKindField, col0.Kind)
	require.Len(t, col0.Children, 1)
	assert.Equal(t, "upcase", col0.Children[0].Name)

	// Column 1: aliased Bar AS baz.
	col1 := query.Children[1]
	assert.Equal(t, "baz", col1.Name)
	assert.Equal(t, protocol.SymbolKindField, col1.Kind)
}

func TestServerDocumentSymbolsUnknownDocument(t *testing.T) {
	server, _ := newTestServer()

	result, err := server.DocumentSymbol(context.Background(), &protocol.DocumentSymbolParams{
		TextDocument: protocol.TextDocumentIdentifier{
			URI: uri.MustParse("file:///tmp/never-opened.vql"),
		},
	})
	require.NoError(t, err)
	require.Empty(t, symbolsFromResult(t, result))
}

func TestServerHoverPluginCall(t *testing.T) {
	server, _ := newTestServer()
	doc_uri := uri.MustParse("file:///tmp/hover.vql")
	document := "SELECT * FROM pslist(pid=1)"

	// DidOpen so the document is known.
	err := server.DidOpen(context.Background(), &protocol.DidOpenTextDocumentParams{
		TextDocument: protocol.TextDocumentItem{
			URI: doc_uri, LanguageID: "vql", Version: 1, Text: document,
		},
	})
	require.NoError(t, err)

	// Cursor on the pslist name (byte 28, line 0).
	hover, err := server.Hover(context.Background(), &protocol.HoverParams{
		TextDocumentPositionParams: protocol.TextDocumentPositionParams{
			TextDocument: protocol.TextDocumentIdentifier{URI: doc_uri},
			Position:     protocol.Position{Line: 0, Character: 14},
		},
	})
	require.NoError(t, err)
	require.NotNil(t, hover)

	content := hoverContents(t, hover)
	assert.Contains(t, content, "**pslist** (plugin)")
	assert.Contains(t, content, "`pid`")
}

func TestServerHoverFunctionCall(t *testing.T) {
	server, _ := newTestServer()
	doc_uri := uri.MustParse("file:///tmp/hover.vql")
	document := "SELECT upcase(str='x') FROM pslist()"

	err := server.DidOpen(context.Background(), &protocol.DidOpenTextDocumentParams{
		TextDocument: protocol.TextDocumentItem{
			URI: doc_uri, LanguageID: "vql", Version: 1, Text: document,
		},
	})
	require.NoError(t, err)

	// Cursor on the upcase name (byte 7).
	hover, err := server.Hover(context.Background(), &protocol.HoverParams{
		TextDocumentPositionParams: protocol.TextDocumentPositionParams{
			TextDocument: protocol.TextDocumentIdentifier{URI: doc_uri},
			Position:     protocol.Position{Line: 0, Character: 7},
		},
	})
	require.NoError(t, err)
	require.NotNil(t, hover)

	content := hoverContents(t, hover)
	assert.Contains(t, content, "**upcase** (function)")
	assert.Contains(t, content, "`str`")
}

func TestServerHoverArgument(t *testing.T) {
	server, _ := newTestServer()
	doc_uri := uri.MustParse("file:///tmp/hover.vql")
	document := "SELECT upcase(str='x') FROM pslist(pid=1)"

	err := server.DidOpen(context.Background(), &protocol.DidOpenTextDocumentParams{
		TextDocument: protocol.TextDocumentItem{
			URI: doc_uri, LanguageID: "vql", Version: 1, Text: document,
		},
	})
	require.NoError(t, err)

	// Cursor on the pid argument name (byte 35) inside pslist().
	hover, err := server.Hover(context.Background(), &protocol.HoverParams{
		TextDocumentPositionParams: protocol.TextDocumentPositionParams{
			TextDocument: protocol.TextDocumentIdentifier{URI: doc_uri},
			Position:     protocol.Position{Line: 0, Character: 35},
		},
	})
	require.NoError(t, err)
	require.NotNil(t, hover)

	content := hoverContents(t, hover)
	assert.Contains(t, content, "**pid**")
	assert.Contains(t, content, "`int`")
}

func TestServerHoverUnknownDocument(t *testing.T) {
	server, _ := newTestServer()

	hover, err := server.Hover(context.Background(), &protocol.HoverParams{
		TextDocumentPositionParams: protocol.TextDocumentPositionParams{
			TextDocument: protocol.TextDocumentIdentifier{
				URI: uri.MustParse("file:///tmp/never-opened.vql"),
			},
			Position: protocol.Position{Line: 0, Character: 0},
		},
	})
	require.NoError(t, err)
	assert.Nil(t, hover)
}

func TestServerHoverUnknownSymbol(t *testing.T) {
	server, _ := newTestServer()
	doc_uri := uri.MustParse("file:///tmp/hover.vql")
	document := "SELECT * FROM pslist() WHERE SomeDynamicColumn > 3"

	err := server.DidOpen(context.Background(), &protocol.DidOpenTextDocumentParams{
		TextDocument: protocol.TextDocumentItem{
			URI: doc_uri, LanguageID: "vql", Version: 1, Text: document,
		},
	})
	require.NoError(t, err)

	// Cursor on the dynamic column (byte 33) - no info to show.
	hover, err := server.Hover(context.Background(), &protocol.HoverParams{
		TextDocumentPositionParams: protocol.TextDocumentPositionParams{
			TextDocument: protocol.TextDocumentIdentifier{URI: doc_uri},
			Position:     protocol.Position{Line: 0, Character: 29},
		},
	})
	require.NoError(t, err)
	assert.Nil(t, hover)
}

// hoverContents extracts the markdown value from a hover result.
func hoverContents(t *testing.T, hover *protocol.Hover) string {
	contents := hover.Contents
	switch v := contents.(type) {
	case *protocol.MarkupContent:
		return v.Value
	case protocol.String:
		return string(v)
	default:
		t.Fatalf("unexpected hover contents type %T", contents)
		return ""
	}
}

// completionLabels extracts the completion item labels from a result.
func completionLabels(t *testing.T, result protocol.CompletionResult) []string {
	slice, ok := result.(protocol.CompletionItemSlice)
	require.True(t, ok, "CompletionResult should be CompletionItemSlice")
	items := []protocol.CompletionItem(slice)
	labels := make([]string, len(items))
	for i, item := range items {
		labels[i] = item.Label
	}
	return labels
}

func TestServerCompletionPluginPrefix(t *testing.T) {
	server, _ := newTestServer()
	doc_uri := uri.MustParse("file:///tmp/completion.vql")
	document := "SELECT * FROM psl"

	err := server.DidOpen(context.Background(), &protocol.DidOpenTextDocumentParams{
		TextDocument: protocol.TextDocumentItem{
			URI: doc_uri, LanguageID: "vql", Version: 1, Text: document,
		},
	})
	require.NoError(t, err)

	result, err := server.Completion(context.Background(), &protocol.CompletionParams{
		TextDocumentPositionParams: protocol.TextDocumentPositionParams{
			TextDocument: protocol.TextDocumentIdentifier{URI: doc_uri},
			Position:     protocol.Position{Line: 0, Character: uint32(len(document))},
		},
	})
	require.NoError(t, err)

	labels := completionLabels(t, result)
	assert.Contains(t, labels, "pslist")
}

func TestServerCompletionFunctionPrefix(t *testing.T) {
	server, _ := newTestServer()
	doc_uri := uri.MustParse("file:///tmp/completion.vql")
	document := "SELECT upc FROM pslist()"

	err := server.DidOpen(context.Background(), &protocol.DidOpenTextDocumentParams{
		TextDocument: protocol.TextDocumentItem{
			URI: doc_uri, LanguageID: "vql", Version: 1, Text: document,
		},
	})
	require.NoError(t, err)

	result, err := server.Completion(context.Background(), &protocol.CompletionParams{
		TextDocumentPositionParams: protocol.TextDocumentPositionParams{
			TextDocument: protocol.TextDocumentIdentifier{URI: doc_uri},
			Position:     protocol.Position{Line: 0, Character: uint32(len(document))},
		},
	})
	require.NoError(t, err)

	labels := completionLabels(t, result)
	assert.Contains(t, labels, "upcase")
}

func TestServerCompletionArtifactPrefix(t *testing.T) {
	server, _ := newTestServer()
	doc_uri := uri.MustParse("file:///tmp/completion.vql")
	document := "SELECT * FROM Art"

	err := server.DidOpen(context.Background(), &protocol.DidOpenTextDocumentParams{
		TextDocument: protocol.TextDocumentItem{
			URI: doc_uri, LanguageID: "vql", Version: 1, Text: document,
		},
	})
	require.NoError(t, err)

	result, err := server.Completion(context.Background(), &protocol.CompletionParams{
		TextDocumentPositionParams: protocol.TextDocumentPositionParams{
			TextDocument: protocol.TextDocumentIdentifier{URI: doc_uri},
			Position:     protocol.Position{Line: 0, Character: uint32(len(document))},
		},
	})
	require.NoError(t, err)

	labels := completionLabels(t, result)
	assert.Contains(t, labels, "Artifact.Linux.Sys.Users")
}

func TestServerCompletionLetVariable(t *testing.T) {
	server, _ := newTestServer()
	doc_uri := uri.MustParse("file:///tmp/completion.vql")
	document := "LET X = 5\nSELECT * FROM "

	err := server.DidOpen(context.Background(), &protocol.DidOpenTextDocumentParams{
		TextDocument: protocol.TextDocumentItem{
			URI: doc_uri, LanguageID: "vql", Version: 1, Text: document,
		},
	})
	require.NoError(t, err)

	result, err := server.Completion(context.Background(), &protocol.CompletionParams{
		TextDocumentPositionParams: protocol.TextDocumentPositionParams{
			TextDocument: protocol.TextDocumentIdentifier{URI: doc_uri},
			Position:     protocol.Position{Line: 1, Character: 14},
		},
	})
	require.NoError(t, err)

	slice, ok := result.(protocol.CompletionItemSlice)
	require.True(t, ok, "CompletionResult should be CompletionItemSlice")
	items := []protocol.CompletionItem(slice)
	found := false
	for _, item := range items {
		if item.Label == "X" && item.Kind == protocol.CompletionItemKindVariable {
			found = true
		}
	}
	assert.True(t, found, "expected LET variable X in completion items")
}

func TestServerCompletionArguments(t *testing.T) {
	server, _ := newTestServer()
	doc_uri := uri.MustParse("file:///tmp/completion.vql")
	document := "SELECT * FROM pslist("

	err := server.DidOpen(context.Background(), &protocol.DidOpenTextDocumentParams{
		TextDocument: protocol.TextDocumentItem{
			URI: doc_uri, LanguageID: "vql", Version: 1, Text: document,
		},
	})
	require.NoError(t, err)

	result, err := server.Completion(context.Background(), &protocol.CompletionParams{
		TextDocumentPositionParams: protocol.TextDocumentPositionParams{
			TextDocument: protocol.TextDocumentIdentifier{URI: doc_uri},
			Position:     protocol.Position{Line: 0, Character: uint32(len(document))},
		},
	})
	require.NoError(t, err)

	slice, ok := result.(protocol.CompletionItemSlice)
	require.True(t, ok, "CompletionResult should be CompletionItemSlice")
	items := []protocol.CompletionItem(slice)
	found := false
	for _, item := range items {
		if item.Label == "pid" && item.Kind == protocol.CompletionItemKindField {
			found = true
		}
	}
	assert.True(t, found, "expected argument pid in completion items")
}

func TestServerCompletionUnknownDocument(t *testing.T) {
	server, _ := newTestServer()
	doc_uri := uri.MustParse("file:///tmp/never-opened.vql")

	result, err := server.Completion(context.Background(), &protocol.CompletionParams{
		TextDocumentPositionParams: protocol.TextDocumentPositionParams{
			TextDocument: protocol.TextDocumentIdentifier{URI: doc_uri},
			Position:     protocol.Position{Line: 0, Character: 0},
		},
	})
	require.NoError(t, err)

	labels := completionLabels(t, result)
	assert.Empty(t, labels)
}
