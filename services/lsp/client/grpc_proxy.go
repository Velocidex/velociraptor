package client

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"go.lsp.dev/protocol"
	"go.lsp.dev/uri"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	api_proto "www.velocidex.com/golang/velociraptor/api/proto"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	"www.velocidex.com/golang/velociraptor/grpc_client"
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

	// When the backend stays unresponsive we keep trying to dial a
	// fresh connection every watchdog cycle. Only if reconnecting
	// keeps failing for this many cycles do we give up and exit,
	// letting the editor client restart us.
	maxReconnectFailures = 10
)

// LSPProxy is a LSPServer that forwards all calls to the gRPC
// endpoint.
type LSPProxy struct {
	protocol.UnimplementedServer

	mu sync.Mutex

	// The api client is accessed from both the forwarding calls and
	// the watchdog goroutine which may swap it for a freshly dialed
	// connection - hence the atomic container.
	api_client atomic.Value // holds api_proto.APIClient

	// documents caches the latest full text of every open document.
	// The backend negotiates full text sync so each didChange
	// carries the complete content, making the cache trivial to
	// maintain. It exists so the proxy can restore backend state if
	// the backend loses it - e.g. when the Velociraptor frontend
	// restarts, its LSP service starts with an empty document map
	// and answers every document operation with NotFoundError until
	// the document is re-opened.
	documents map[uri.URI]docState

	// Connection parameters needed to dial a fresh backend
	// connection when the current one becomes unresponsive.
	identity   grpc_client.CallerIdentity
	config_obj *config_proto.Config

	log_file *os.File
}

// docState is the cached state of an open document.
type docState struct {
	text    string
	version int32
}

func (self *LSPProxy) getAPIClient() api_proto.APIClient {
	return self.api_client.Load().(api_proto.APIClient)
}

// docURIFromParams extracts the document URI from any params type
// that refers to a document, so recovery logic can work generically.
func docURIFromParams(params any) (uri.URI, bool) {
	switch p := params.(type) {
	case *protocol.DidChangeTextDocumentParams:
		return p.TextDocument.URI, true
	case *protocol.CompletionParams:
		return p.TextDocument.URI, true
	case *protocol.HoverParams:
		return p.TextDocument.URI, true
	case *protocol.SignatureHelpParams:
		return p.TextDocument.URI, true
	case *protocol.DocumentSymbolParams:
		return p.TextDocument.URI, true
	case *protocol.FoldingRangeParams:
		return p.TextDocument.URI, true
	case *protocol.CodeActionParams:
		return p.TextDocument.URI, true
	case *protocol.SemanticTokensParams:
		return p.TextDocument.URI, true
	case *protocol.InlayHintParams:
		return p.TextDocument.URI, true
	case *protocol.DocumentFormattingParams:
		return p.TextDocument.URI, true
	case *protocol.DocumentHighlightParams:
		return p.TextDocument.URI, true
	case *protocol.ReferenceParams:
		return p.TextDocument.URI, true
	case *protocol.PrepareRenameParams:
		return p.TextDocument.URI, true
	case *protocol.RenameParams:
		return p.TextDocument.URI, true
	}
	return "", false
}

// resyncDocument restores backend state for a document the backend
// no longer knows about by replaying its cached content as a
// didOpen. Best effort - failures are logged and surfaced to the
// caller through the retry that follows.
func (self *LSPProxy) resyncDocument(ctx context.Context, uri uri.URI) {
	state, ok := self.documents[uri]
	if !ok {
		self.Debug("Recovery: no cached content for %v", uri)
		return
	}
	self.Debug("Recovery: replaying didOpen for %v", uri)
	diagnostics := []protocol.Diagnostic{}
	err := self.forwardCall(ctx, DidOpenOp, 0,
		&protocol.DidOpenTextDocumentParams{
			TextDocument: protocol.TextDocumentItem{
				URI:        uri,
				LanguageID: "vql",
				Version:    state.version,
				Text:       state.text,
			},
		}, &diagnostics)
	if err != nil {
		self.Debug("Recovery: didOpen failed: %v", err)
	}
}

// forwardCallWithRecovery forwards a call and, when the backend
// reports it does not know the document (NotFoundError - typically
// because the frontend restarted and lost its LSP state), replays
// the cached document content and retries once. All document scoped
// handlers should use this instead of forwardCall directly.
func (self *LSPProxy) forwardCallWithRecovery(
	ctx context.Context,
	operation string,
	id uint32,
	params any, result any) error {

	err := self.forwardCall(ctx, operation, id, params, result)
	if err == nil || !strings.Contains(err.Error(), "NotFoundError") {
		return err
	}

	uri, ok := docURIFromParams(params)
	if !ok {
		return err
	}

	self.resyncDocument(ctx, uri)
	return self.forwardCall(ctx, operation, id, params, result)
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
	resp, err := self.getAPIClient().LSP(ctx, &api_proto.LSPRequest{
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
	err := self.forwardCallWithRecovery(ctx, CompletionOp, 0, params, &result)
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
	err := self.forwardCallWithRecovery(ctx, HoverOp, 0, params, result)
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

	// Cache the content so it can be replayed if the backend loses
	// its state.
	self.documents[params.TextDocument.URI] = docState{
		text:    params.TextDocument.Text,
		version: params.TextDocument.Version,
	}

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

	// Update the cache. Sync kind is Full so the whole-document
	// change event carries the complete document text.
	for _, change := range params.ContentChanges {
		if whole, ok := change.(*protocol.TextDocumentContentChangeWholeDocument); ok {
			self.documents[params.TextDocument.URI] = docState{
				text:    whole.Text,
				version: params.TextDocument.Version,
			}
		}
	}

	diagnostics := []protocol.Diagnostic{}
	uri := params.TextDocument.URI
	err := self.forwardCallWithRecovery(ctx, DidChangeOp, 0, params, &diagnostics)
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

	// The document is gone - drop the cached content.
	delete(self.documents, params.TextDocument.URI)

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
	err := self.forwardCallWithRecovery(ctx, FormattingOp, 0, params, &result)
	if err != nil {
		return nil, err
	}
	return result, nil
}

func (self *LSPProxy) SignatureHelp(
	ctx context.Context, params *protocol.SignatureHelpParams) (
	*protocol.SignatureHelp, error) {
	result := &protocol.SignatureHelp{}
	err := self.forwardCallWithRecovery(ctx, SignatureHelpOp, 0, params, result)
	if err != nil {
		return nil, err
	}
	return result, nil
}

func (self *LSPProxy) FoldingRanges(
	ctx context.Context, params *protocol.FoldingRangeParams) (
	[]protocol.FoldingRange, error) {
	result := []protocol.FoldingRange{}
	err := self.forwardCallWithRecovery(ctx, FoldingRangesOp, 0, params, &result)
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
	err := self.forwardCallWithRecovery(ctx, DocumentSymbolsOp, 0, params, &result)
	if err != nil {
		return nil, err
	}
	return protocol.DocumentSymbolSlice(result), nil
}

func (self *LSPProxy) InlayHint(
	ctx context.Context, params *protocol.InlayHintParams) (
	[]protocol.InlayHint, error) {
	result := []protocol.InlayHint{}
	err := self.forwardCallWithRecovery(ctx, InlayHintOp, 0, params, &result)
	if err != nil {
		return nil, err
	}
	return result, nil
}

func (self *LSPProxy) CodeAction(
	ctx context.Context, params *protocol.CodeActionParams) (
	[]protocol.CommandOrCodeAction, error) {
	result := []protocol.CommandOrCodeAction{}
	err := self.forwardCallWithRecovery(ctx, CodeActionOp, 0, params, &result)
	if err != nil {
		return nil, err
	}
	return result, nil
}

func (self *LSPProxy) References(
	ctx context.Context, params *protocol.ReferenceParams) (
	[]protocol.Location, error) {
	result := []protocol.Location{}
	err := self.forwardCallWithRecovery(ctx, ReferencesOp, 0, params, &result)
	if err != nil {
		return nil, err
	}
	return result, nil
}

func (self *LSPProxy) PrepareRename(
	ctx context.Context, params *protocol.PrepareRenameParams) (
	protocol.PrepareRenameResult, error) {
	result := protocol.Range{}
	err := self.forwardCallWithRecovery(ctx, PrepareRenameOp, 0, params, &result)
	if err != nil {
		return nil, err
	}
	return &result, nil
}

func (self *LSPProxy) Rename(
	ctx context.Context, params *protocol.RenameParams) (
	*protocol.WorkspaceEdit, error) {
	result := &protocol.WorkspaceEdit{}
	err := self.forwardCallWithRecovery(ctx, RenameOp, 0, params, result)
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
	return result, self.forwardCallWithRecovery(ctx, DiagnosticOp, 0, params, result)
}

func (self *LSPProxy) DocumentHighlight(
	ctx context.Context, params *protocol.DocumentHighlightParams) (
	[]protocol.DocumentHighlight, error) {

	self.mu.Lock()
	defer self.mu.Unlock()

	result := []protocol.DocumentHighlight{}
	return result, self.forwardCallWithRecovery(ctx, DocumentHighlightOp, 0, params, &result)
}

func (self *LSPProxy) WorkDoneProgressCancel(
	ctx context.Context,
	params *protocol.WorkDoneProgressCancelParams) error {
	return nil
}

// Shutdown is part of the LSP lifecycle - the editor client sends it
// before stopping the server. Acknowledge it so the client does not
// have to time out and hard kill us.
func (self *LSPProxy) Shutdown(ctx context.Context) error {
	return nil
}

// Exit terminates the process after a Shutdown.
func (self *LSPProxy) Exit(ctx context.Context) error {
	os.Exit(0)
	return nil
}

// watchdog keeps the connection to the backend healthy.
//
// When the Velociraptor frontend behind this proxy dies or restarts,
// the gRPC channel can silently black hole every call while looking
// perfectly alive at the transport level - from the editor's point
// of view requests are simply never answered. The watchdog pings the
// backend periodically and, once pings keep timing out, dials a fresh
// connection and swaps it in without dropping the editor session.
// This matters because a proxy restart would lose all document state
// (the editor does not resend didOpen for files it already opened),
// leaving completions and diagnostics dead until the file is
// reopened.
//
// If dialing fresh connections keeps failing as well we exit so the
// editor client can restart us through its own restart logic.
func (self *LSPProxy) watchdog(ctx context.Context) {
	failures := 0
	reconnect_failures := 0
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
			_, err := self.getAPIClient().LSP(ping_ctx,
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
			if failures < maxWatchdogFailures {
				continue
			}

			// The connection is stuck - dial a brand new one
			// and swap it in. The pool never re-issues the
			// connection we hold (its closer is a no-op) so
			// this always results in a freshly dialed channel.
			reconnect_ctx, reconnect_cancel := context.WithTimeout(
				context.Background(), watchdogTimeout)
			new_client, _, err := grpc_client.Factory.GetAPIClient(
				reconnect_ctx, self.identity, self.config_obj)
			reconnect_cancel()
			if err == nil {
				self.api_client.Store(new_client)
				self.Debug("Watchdog: reconnected to backend")
				failures = 0
				reconnect_failures = 0
				continue
			}

			// Reconnecting failed too - keep trying on future
			// cycles until we exceed our budget, then exit so
			// the editor client can restart us.
			reconnect_failures++
			failures = 0
			self.Debug("Watchdog: reconnect failed (%v/%v): %v",
				reconnect_failures, maxReconnectFailures, err)
			if reconnect_failures >= maxReconnectFailures {
				self.Debug("Watchdog: giving up, exiting so the client can reconnect")
				os.Exit(1)
			}
		}
	}
}

func NewLSPProxy(
	ctx context.Context,
	api_client api_proto.APIClient,
	identity grpc_client.CallerIdentity,
	config_obj *config_proto.Config,
	log_file *os.File) protocol.Server {
	res := &LSPProxy{
		documents:  make(map[uri.URI]docState),
		identity:   identity,
		config_obj: config_obj,
		log_file:   log_file,
	}
	res.api_client.Store(api_client)
	go res.watchdog(ctx)
	return res
}
