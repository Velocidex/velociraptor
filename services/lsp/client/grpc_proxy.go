package client

import (
	"context"
	"errors"
	"fmt"
	"os"
	"sync"
	"time"

	"go.lsp.dev/protocol"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	api_proto "www.velocidex.com/golang/velociraptor/api/proto"
	"www.velocidex.com/golang/velociraptor/utils"
)

const (
	InitializeOp = "Initialize"
	DidOpenOp    = "DidOpen"
	DidChangeOp  = "DidChange"
	DidCloseOp   = "DidClose"
	CompletionOp = "CompletionOp"
	HoverOp      = "HoverOp"

	DiagnosticOp = "DiagnosticOp"

	FormattingOp = "FormattingOp"

	SignatureHelpOp = "SignatureHelpOp"

	FoldingRangesOp = "FoldingRangesOp"

	WorkspaceSymbolsOp = "WorkspaceSymbolsOp"

	DocumentSymbolsOp = "DocumentSymbolsOp"

	InlayHintOp = "InlayHintOp"

	CodeActionOp = "CodeActionOp"

	ReferencesOp = "ReferencesOp"

	PrepareRenameOp = "PrepareRenameOp"

	RenameOp = "RenameOp"

	SemanticTokensOp = "SemanticTokensOp"

	SymbolOp            = "SymbolOp"
	DocumentHighlightOp = "DocumentHighlightOp"
)

var (
	ErrorLspClientContextMissing = errors.New("ErrorLspClientContextMissing")

	// Hard cap on a single forwarded call. When the backend dies
	// the gRPC channel can silently black hole calls (the transport
	// looks alive but nothing answers) - without a deadline the
	// editor just spins forever.
	forwardCallTimeout = 30 * time.Second

	// Watchdog tuning. A healthy backend answers pings in
	// milliseconds so only a genuinely stuck connection will
	// accumulate consecutive timeouts.
	watchdogInterval    = 10 * time.Second
	watchdogTimeout     = 10 * time.Second
	maxWatchdogFailures = 3
)

// LSPProxy is a LSPServer that forwards all calls to the gRPC
// endpoint.
type LSPProxy struct {
	protocol.UnimplementedServer

	mu         sync.Mutex
	api_client api_proto.APIClient
	log_file   *os.File
}

func (self *LSPProxy) Debug(format string, v ...interface{}) {
	if self.log_file == nil {
		return
	}
	fmt.Fprintf(self.log_file, "\n"+format+"\n", v...)
}

func (self *LSPProxy) forwardCall(
	ctx context.Context,
	operation string,
	id uint32,
	params any, result any) error {

	// Never let a call hang on a dead connection - see
	// forwardCallTimeout above.
	ctx, cancel := context.WithTimeout(ctx, forwardCallTimeout)
	defer cancel()

	serialized, err := protocol.Marshal(params)
	if err != nil {
		return err
	}

	self.Debug("Grpc call %v with id %v", operation, id)
	resp, err := self.api_client.LSP(ctx, &api_proto.LSPRequest{
		Operation: operation,
		Id:        uint32(id),
		Json:      string(serialized),
	})
	if err != nil {
		self.Debug("Grpc call failed %v: %v", id, err.Error())
		return err
	}
	self.Debug("Grpc call succeeded %v!", id)
	if result == nil {
		return nil
	}

	err = protocol.Unmarshal([]byte(resp.Json), result)
	if err != nil {
		self.Debug("Grpc call failed %v: %v", id, err.Error())
		return err
	}
	return nil
}

func (self *LSPProxy) Initialize(
	ctx context.Context,
	params *protocol.InitializeParams) (*protocol.InitializeResult, error) {
	result := &protocol.InitializeResult{}
	return result, self.forwardCall(ctx, InitializeOp, 0, params, result)
}

func (self *LSPProxy) Completion(
	ctx context.Context,
	params *protocol.CompletionParams) (protocol.CompletionResult, error) {

	self.mu.Lock()
	defer self.mu.Unlock()

	result := []protocol.CompletionItem{}
	err := self.forwardCall(ctx, CompletionOp, 0, params, &result)
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
	err := self.forwardCall(ctx, HoverOp, 0, params, result)
	if err != nil {
		return nil, err
	}

	// The server returns a null result when there is nothing to
	// show. Unmarshalling null into the pre-allocated struct leaves
	// it with empty contents which crashes editor clients when they
	// convert the response - convert it back to a null result.
	if result.Contents == nil {
		return nil, nil
	}
	return result, nil
}

func (self *LSPProxy) DidOpen(
	ctx context.Context, params *protocol.DidOpenTextDocumentParams) error {

	self.mu.Lock()
	defer self.mu.Unlock()

	diagnostics := []protocol.Diagnostic{}
	uri := params.TextDocument.URI
	err := self.forwardCall(ctx, DidOpenOp, 0, params, &diagnostics)
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

func (self *LSPProxy) DidChange(
	ctx context.Context, params *protocol.DidChangeTextDocumentParams) error {

	self.mu.Lock()
	defer self.mu.Unlock()

	diagnostics := []protocol.Diagnostic{}
	uri := params.TextDocument.URI
	err := self.forwardCall(ctx, DidChangeOp, 0, params, &diagnostics)
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

func (self *LSPProxy) DidClose(
	ctx context.Context, params *protocol.DidCloseTextDocumentParams) error {

	self.mu.Lock()
	defer self.mu.Unlock()

	diagnostics := []protocol.Diagnostic{}
	uri := params.TextDocument.URI
	err := self.forwardCall(ctx, DidCloseOp, 0, params, &diagnostics)
	if err != nil {
		return err
	}

	// Unsolicited publication of the (empty) diagnostics to clear
	// the ones from the closed document.
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
	err := self.forwardCall(ctx, FormattingOp, 0, params, &result)
	if err != nil {
		return nil, err
	}
	return result, nil
}

func (self *LSPProxy) SignatureHelp(
	ctx context.Context, params *protocol.SignatureHelpParams) (
	*protocol.SignatureHelp, error) {
	result := &protocol.SignatureHelp{}
	err := self.forwardCall(ctx, SignatureHelpOp, 0, params, result)
	if err != nil {
		return nil, err
	}
	return result, nil
}

func (self *LSPProxy) FoldingRanges(
	ctx context.Context, params *protocol.FoldingRangeParams) (
	[]protocol.FoldingRange, error) {
	result := []protocol.FoldingRange{}
	err := self.forwardCall(ctx, FoldingRangesOp, 0, params, &result)
	if err != nil {
		return nil, err
	}
	return result, nil
}

func (self *LSPProxy) Symbols(
	ctx context.Context, params *protocol.WorkspaceSymbolParams) (
	protocol.WorkspaceSymbolResult, error) {
	result := []protocol.WorkspaceSymbol{}
	err := self.forwardCall(ctx, WorkspaceSymbolsOp, 0, params, &result)
	if err != nil {
		return nil, err
	}
	return protocol.WorkspaceSymbolSlice(result), nil
}

func (self *LSPProxy) DocumentSymbol(
	ctx context.Context, params *protocol.DocumentSymbolParams) (
	protocol.DocumentSymbolResult, error) {
	result := []protocol.DocumentSymbol{}
	err := self.forwardCall(ctx, DocumentSymbolsOp, 0, params, &result)
	if err != nil {
		return nil, err
	}
	return protocol.DocumentSymbolSlice(result), nil
}

func (self *LSPProxy) InlayHint(
	ctx context.Context, params *protocol.InlayHintParams) (
	[]protocol.InlayHint, error) {
	result := []protocol.InlayHint{}
	err := self.forwardCall(ctx, InlayHintOp, 0, params, &result)
	if err != nil {
		return nil, err
	}
	return result, nil
}

func (self *LSPProxy) CodeAction(
	ctx context.Context, params *protocol.CodeActionParams) (
	[]protocol.CommandOrCodeAction, error) {
	result := []protocol.CommandOrCodeAction{}
	err := self.forwardCall(ctx, CodeActionOp, 0, params, &result)
	if err != nil {
		return nil, err
	}
	return result, nil
}

func (self *LSPProxy) References(
	ctx context.Context, params *protocol.ReferenceParams) (
	[]protocol.Location, error) {
	result := []protocol.Location{}
	err := self.forwardCall(ctx, ReferencesOp, 0, params, &result)
	if err != nil {
		return nil, err
	}
	return result, nil
}

func (self *LSPProxy) PrepareRename(
	ctx context.Context, params *protocol.PrepareRenameParams) (
	protocol.PrepareRenameResult, error) {
	result := protocol.Range{}
	err := self.forwardCall(ctx, PrepareRenameOp, 0, params, &result)
	if err != nil {
		return nil, err
	}
	return &result, nil
}

func (self *LSPProxy) Rename(
	ctx context.Context, params *protocol.RenameParams) (
	*protocol.WorkspaceEdit, error) {
	result := &protocol.WorkspaceEdit{}
	err := self.forwardCall(ctx, RenameOp, 0, params, result)
	if err != nil {
		return nil, err
	}
	return result, nil
}

func (self *LSPProxy) SemanticTokensFull(
	ctx context.Context, params *protocol.SemanticTokensParams) (
	*protocol.SemanticTokens, error) {

	self.mu.Lock()
	defer self.mu.Unlock()

	serialized, _ := protocol.Marshal(params)
	id := uint32(utils.GetId())
	self.Debug("SemanticTokens %v id %v", string(serialized), id)

	result := &protocol.SemanticTokens{}
	return result, self.forwardCall(
		ctx, SemanticTokensOp, id, params, result)
}

func (self *LSPProxy) Diagnostic(ctx context.Context,
	params *protocol.DocumentDiagnosticParams) (
	protocol.DocumentDiagnosticReport, error) {

	self.mu.Lock()
	defer self.mu.Unlock()

	result := &protocol.RelatedFullDocumentDiagnosticReport{}
	return result, self.forwardCall(ctx, DiagnosticOp, 0, params, result)
}

func (self *LSPProxy) DocumentHighlight(
	ctx context.Context, params *protocol.DocumentHighlightParams) (
	[]protocol.DocumentHighlight, error) {

	self.mu.Lock()
	defer self.mu.Unlock()

	result := []protocol.DocumentHighlight{}
	return result, self.forwardCall(ctx, DocumentHighlightOp, 0, params, &result)
}

func (self *LSPProxy) WorkDoneProgressCancel(
	ctx context.Context,
	params *protocol.WorkDoneProgressCancelParams) error {
	return nil
}

// watchdog detects a dead backend so this process can exit and let
// the editor client restart it with a fresh connection.
//
// Editor clients (e.g. vscode-languageclient) only restart the
// language server when the process exits. When the Velociraptor
// frontend behind this proxy dies or restarts, the gRPC channel can
// silently black hole every call while the process stays alive -
// from the client's point of view requests are simply never answered.
// Pinging the backend periodically and exiting when it stays
// unresponsive turns such a hang into a clean automatic restart.
func (self *LSPProxy) watchdog(ctx context.Context) {
	failures := 0
	ticker := time.NewTicker(watchdogInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return

		case <-ticker.C:
			// The ping targets a document that was never
			// opened - the backend answers with an empty
			// diagnostics list without touching anything.
			ping_ctx, cancel := context.WithTimeout(
				context.Background(), watchdogTimeout)
			_, err := self.api_client.LSP(ping_ctx,
				&api_proto.LSPRequest{
					Operation: DidCloseOp,
					Json:      "{}",
				})
			cancel()

			// Only timeouts indicate a stuck connection -
			// the transport looks alive but nothing answers.
			// Any other outcome (success, fast refusal,
			// connection down) either proves the backend is
			// responsive or is something gRPC retries on its
			// own (e.g. Unavailable while the frontend
			// restarts), so those reset the failure count.
			if err == nil ||
				status.Code(err) != codes.DeadlineExceeded {
				if err != nil {
					self.Debug("Watchdog: ping: %v", err)
				}
				failures = 0
				continue
			}

			failures++
			self.Debug("Watchdog: backend unresponsive (%v/%v)",
				failures, maxWatchdogFailures)
			if failures >= maxWatchdogFailures {
				self.Debug("Watchdog: giving up, exiting so the client can reconnect")
				os.Exit(1)
			}
		}
	}
}

func NewLSPProxy(
	ctx context.Context,
	api_client api_proto.APIClient, log_file *os.File) protocol.Server {
	res := &LSPProxy{
		api_client: api_client,
		log_file:   log_file,
	}
	go res.watchdog(ctx)
	return res
}
