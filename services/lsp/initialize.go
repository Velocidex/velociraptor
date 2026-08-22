package lsp

import (
	"context"

	"go.lsp.dev/protocol"
)

func (self *LSPServer) Initialize(
	ctx context.Context,
	params *protocol.InitializeParams) (*protocol.InitializeResult, error) {
	return &protocol.InitializeResult{
		Capabilities: protocol.ServerCapabilities{
			// Sync whole documents on open/change.
			TextDocumentSync: protocol.TextDocumentSyncKindFull,

			// The server pushes diagnostics via
			// textDocument/publishDiagnostics. Pull diagnostics
			// (textDocument/diagnostic) are deliberately NOT
			// advertised: clients that see the capability register
			// an extra diagnostic collection and the same
			// diagnostics appear twice in the problems panel.

			// The server provides a document outline.
			DocumentSymbolProvider: protocol.Boolean(true),

			// The server provides hover documentation.
			HoverProvider: protocol.Boolean(true),

			// The server provides autocompletion. Typing '.' (as in
			// Artifact.Linux.Sys) triggers completion. '(' also
			// triggers completion so argument names pop up right
			// after an unclosed call like "pslist(" - such queries do
			// not parse yet so signature help can not fire for them
			// and the two popups never overlap in practice.
			CompletionProvider: &protocol.CompletionOptions{
				TriggerCharacters: []string{".", "(", "?"},
			},

			// The server provides semantic highlighting.
			SemanticTokensProvider: &protocol.SemanticTokensOptions{
				Legend: protocol.SemanticTokensLegend{
					TokenTypes:     tokenTypesLegend,
					TokenModifiers: tokenModifiersLegend,
				},
				Full: protocol.Boolean(true),
			},

			// The server provides document formatting.
			DocumentFormattingProvider: protocol.Boolean(true),

			// The server provides signature help.
			SignatureHelpProvider: &protocol.SignatureHelpOptions{
				TriggerCharacters: []string{"(", ","},
			},

			// The server provides folding ranges.
			FoldingRangeProvider: protocol.Boolean(true),

			// The server provides workspace symbols.
			WorkspaceSymbolProvider: protocol.Boolean(true),

			// The server provides code actions.
			CodeActionProvider: protocol.Boolean(true),

			// The server provides references.
			ReferencesProvider: protocol.Boolean(true),

			// The server provides rename.
			RenameProvider: protocol.Boolean(true),

			// The server provides semantic highlighting.
			SemanticTokensProvider: &protocol.SemanticTokensOptions{
				Legend: protocol.SemanticTokensLegend{
					TokenTypes:     tokenTypesLegend,
					TokenModifiers: tokenModifiersLegend,
				},
				Full: protocol.Boolean(true),
			},
		},
		ServerInfo: protocol.ServerInfo{
			Name:    "vql-lsp",
			Version: protocol.NewOptional("0.1"),
		},
	}, nil
}
