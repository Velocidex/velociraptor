package client

import (
	"context"
	"errors"
	"sync"

	"go.lsp.dev/protocol"
	api_proto "www.velocidex.com/golang/velociraptor/api/proto"
)

const (
	InitializeOp = "Initialize"
	DidOpenOp    = "DidOpen"
	CompletionOp = "CompletionOp"
	HoverOp      = "HoverOp"
	DiagnosticOp = "DiagnosticOp"
	SymbolOp     = "SymbolOp"
)

var (
	ErrorLspClientContextMissing = errors.New("ErrorLspClientContextMissing")
)

// LSPPRoxy is a LSPServer that forwards all calls to the gRPC
// endpoint.
type LSPProxy struct {
	protocol.UnimplementedServer

	mu         sync.Mutex
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

	self.mu.Lock()
	defer self.mu.Unlock()

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

	self.mu.Lock()
	defer self.mu.Unlock()

	result := &protocol.Hover{}
	return result, self.forwardCall(ctx, HoverOp, params, result)
}

func (self *LSPProxy) DidOpen(
	ctx context.Context, params *protocol.DidOpenTextDocumentParams) error {

	self.mu.Lock()
	defer self.mu.Unlock()

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

func (self *LSPProxy) Diagnostic(ctx context.Context,
	params *protocol.DocumentDiagnosticParams) (
	protocol.DocumentDiagnosticReport, error) {

	self.mu.Lock()
	defer self.mu.Unlock()

	result := &protocol.RelatedFullDocumentDiagnosticReport{}
	return result, self.forwardCall(ctx, DiagnosticOp, params, result)
}

func (self *LSPProxy) DocumentSymbol(
	ctx context.Context, params *protocol.DocumentSymbolParams) (
	protocol.DocumentSymbolResult, error) {

	self.mu.Lock()
	defer self.mu.Unlock()

	result := &protocol.DocumentSymbolSlice{}
	return result, self.forwardCall(ctx, SymbolOp, params, result)
}

func NewLSPProxy(api_client api_proto.APIClient) protocol.Server {
	return &LSPProxy{
		api_client: api_client,
	}
}
