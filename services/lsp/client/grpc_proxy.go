package client

import (
	"context"
	"errors"

	"go.lsp.dev/protocol"
	api_proto "www.velocidex.com/golang/velociraptor/api/proto"
)

const (
	InitializeOp = "Initialize"
	DidOpenOp    = "DidOpen"
	CompletionOp = "CompletionOp"
	HoverOp      = "HoverOp"
	FormattingOp = "FormattingOp"

	SignatureHelpOp = "SignatureHelpOp"

	FoldingRangesOp = "FoldingRangesOp"

	WorkspaceSymbolsOp = "WorkspaceSymbolsOp"

	InlayHintOp = "InlayHintOp"

	CodeActionOp = "CodeActionOp"

	ReferencesOp = "ReferencesOp"

	PrepareRenameOp = "PrepareRenameOp"

	RenameOp = "RenameOp"

	SemanticTokensOp = "SemanticTokensOp"
)

var (
	ErrorLspClientContextMissing = errors.New("ErrorLspClientContextMissing")
)

// LSPPRoxy is a LSPServer that forwards all calls to the gRPC
// endpoint.
type LSPProxy struct {
	protocol.UnimplementedServer

	api_client api_proto.APIClient
}

func (self *LSPProxy) forwardCall(
	ctx context.Context,
	operation string,
	params any, result any) error {
	serialized, err := protocol.Marshal(params)
	if err != nil {
		return err
	}

	resp, err := self.api_client.LSP(ctx, &api_proto.LSPRequest{
		Operation: operation,
		Json:      string(serialized),
	})
	if err != nil {
		return err
	}

	if result == nil {
		return nil
	}

	return protocol.Unmarshal([]byte(resp.Json), result)
}

func (self *LSPProxy) Initialize(
	ctx context.Context,
	params *protocol.InitializeParams) (*protocol.InitializeResult, error) {
	result := &protocol.InitializeResult{}
	return result, self.forwardCall(ctx, InitializeOp, params, result)
}

func (self *LSPProxy) Completion(
	ctx context.Context,
	params *protocol.CompletionParams) (protocol.CompletionResult, error) {
	result := []protocol.CompletionItem{}
	err := self.forwardCall(ctx, CompletionOp, params, &result)
	if err != nil {
		return nil, err
	}
	return protocol.CompletionItemSlice(result), nil
}

func (self *LSPProxy) Hover(
	ctx context.Context, params *protocol.HoverParams) (
	*protocol.Hover, error) {
	result := &protocol.Hover{}
	return result, self.forwardCall(ctx, HoverOp, params, result)
}

func (self *LSPProxy) DidOpen(
	ctx context.Context, params *protocol.DidOpenTextDocumentParams) error {

	diagnostics := []protocol.Diagnostic{}
	uri := params.TextDocument.URI
	err := self.forwardCall(ctx, DidOpenOp, params, &diagnostics)
	if err != nil {
		return err
	}

	// Unsolicited publication of the diagnostics.
	lsp_client, ok := protocol.ClientFromContext(ctx)
	if !ok {
		return ErrorLspClientContextMissing
	}

	return lsp_client.PublishDiagnostics(ctx,
		&protocol.PublishDiagnosticsParams{
			URI:         uri,
			Diagnostics: diagnostics,
		})
}

func (self *LSPProxy) Formatting(
	ctx context.Context, params *protocol.DocumentFormattingParams) (
	[]protocol.TextEdit, error) {
	result := []protocol.TextEdit{}
	err := self.forwardCall(ctx, FormattingOp, params, &result)
	if err != nil {
		return nil, err
	}
	return result, nil
}

func (self *LSPProxy) SignatureHelp(
	ctx context.Context, params *protocol.SignatureHelpParams) (
	*protocol.SignatureHelp, error) {
	result := &protocol.SignatureHelp{}
	err := self.forwardCall(ctx, SignatureHelpOp, params, result)
	if err != nil {
		return nil, err
	}
	return result, nil
}

func (self *LSPProxy) FoldingRanges(
	ctx context.Context, params *protocol.FoldingRangeParams) (
	[]protocol.FoldingRange, error) {
	result := []protocol.FoldingRange{}
	err := self.forwardCall(ctx, FoldingRangesOp, params, &result)
	if err != nil {
		return nil, err
	}
	return result, nil
}

func (self *LSPProxy) Symbols(
	ctx context.Context, params *protocol.WorkspaceSymbolParams) (
	protocol.WorkspaceSymbolResult, error) {
	result := []protocol.WorkspaceSymbol{}
	err := self.forwardCall(ctx, WorkspaceSymbolsOp, params, &result)
	if err != nil {
		return nil, err
	}
	return protocol.WorkspaceSymbolSlice(result), nil
}

func (self *LSPProxy) InlayHint(
	ctx context.Context, params *protocol.InlayHintParams) (
	[]protocol.InlayHint, error) {
	result := []protocol.InlayHint{}
	err := self.forwardCall(ctx, InlayHintOp, params, &result)
	if err != nil {
		return nil, err
	}
	return result, nil
}

func (self *LSPProxy) CodeAction(
	ctx context.Context, params *protocol.CodeActionParams) (
	[]protocol.CommandOrCodeAction, error) {
	result := []protocol.CommandOrCodeAction{}
	err := self.forwardCall(ctx, CodeActionOp, params, &result)
	if err != nil {
		return nil, err
	}
	return result, nil
}

func (self *LSPProxy) References(
	ctx context.Context, params *protocol.ReferenceParams) (
	[]protocol.Location, error) {
	result := []protocol.Location{}
	err := self.forwardCall(ctx, ReferencesOp, params, &result)
	if err != nil {
		return nil, err
	}
	return result, nil
}

func (self *LSPProxy) PrepareRename(
	ctx context.Context, params *protocol.PrepareRenameParams) (
	protocol.PrepareRenameResult, error) {
	result := protocol.Range{}
	err := self.forwardCall(ctx, PrepareRenameOp, params, &result)
	if err != nil {
		return nil, err
	}
	return &result, nil
}

func (self *LSPProxy) Rename(
	ctx context.Context, params *protocol.RenameParams) (
	*protocol.WorkspaceEdit, error) {
	result := &protocol.WorkspaceEdit{}
	err := self.forwardCall(ctx, RenameOp, params, result)
	if err != nil {
		return nil, err
	}
	return result, nil
}

func (self *LSPProxy) SemanticTokensFull(
	ctx context.Context, params *protocol.SemanticTokensParams) (
	*protocol.SemanticTokens, error) {
	result := &protocol.SemanticTokens{}
	err := self.forwardCall(ctx, SemanticTokensOp, params, result)
	if err != nil {
		return nil, err
	}
	return result, nil
}

func NewLSPProxy(api_client api_proto.APIClient) protocol.Server {
	return &LSPProxy{
		api_client: api_client,
	}
}
