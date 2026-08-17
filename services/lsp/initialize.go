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
			// textDocument/publishDiagnostics.
			DiagnosticProvider: &protocol.DiagnosticOptions{
				InterFileDependencies: false,
				WorkspaceDiagnostics:  false,
				Identifier:            new("vql"),
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

			// The server provides inlay hints.
			InlayHintProvider: protocol.Boolean(true),

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
