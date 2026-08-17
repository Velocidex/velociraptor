package lsp

import (
	"context"
	"fmt"
	"sync"

	"go.lsp.dev/protocol"
	"go.lsp.dev/uri"
	api_proto "www.velocidex.com/golang/velociraptor/api/proto"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	"www.velocidex.com/golang/velociraptor/services"
	"www.velocidex.com/golang/velociraptor/services/lsp/client"
	"www.velocidex.com/golang/velociraptor/utils"
)

type LSPServer struct {
	config_obj *config_proto.Config

	// TODO Replace with LRU
	mu        sync.Mutex
	documents map[uri.URI]*Document
}

func (self *LSPServer) returnRespose(resp any) (*api_proto.LSPResponse, error) {
	serialized, err := protocol.Marshal(resp)
	if err != nil {
		return nil, err
	}

	fmt.Printf("Got %v\n", string(serialized))

	return &api_proto.LSPResponse{
		Json: string(serialized),
	}, nil
}

// Main dispatch for the lsp server.
func (self *LSPServer) LSP(
	ctx context.Context,
	in *api_proto.LSPRequest) (*api_proto.LSPResponse, error) {
	switch in.Operation {
	case client.InitializeOp:
		req := &protocol.InitializeParams{}
		err := protocol.Unmarshal([]byte(in.Json), req)
		if err != nil {
			return nil, err
		}

		result, err := self.Initialize(ctx, req)
		if err != nil {
			return nil, err
		}

		return self.returnRespose(result)

	case client.DidOpenOp:
		req := &protocol.DidOpenTextDocumentParams{}
		err := protocol.Unmarshal([]byte(in.Json), req)
		if err != nil {
			return nil, err
		}

		result, err := self.DidOpen(ctx, req)
		if err != nil {
			return nil, err
		}

		return self.returnRespose(result)

	case client.CompletionOp:
		req := &protocol.CompletionParams{}
		err := protocol.Unmarshal([]byte(in.Json), req)
		if err != nil {
			return nil, err
		}

		result, err := self.Completion(ctx, req)
		if err != nil {
			return nil, err
		}
		return self.returnRespose(result)

	case client.HoverOp:
		req := &protocol.HoverParams{}
		err := protocol.Unmarshal([]byte(in.Json), req)
		if err != nil {
			return nil, err
		}

		result, err := self.Hover(ctx, req)
		if err != nil {
			return nil, err
		}
		return self.returnRespose(result)

	case client.FormattingOp:
		req := &protocol.DocumentFormattingParams{}
		err := protocol.Unmarshal([]byte(in.Json), req)
		if err != nil {
			return nil, err
		}

		result, err := self.Formatting(ctx, req)
		if err != nil {
			return nil, err
		}
		return self.returnRespose(result)

	case client.SignatureHelpOp:
		req := &protocol.SignatureHelpParams{}
		err := protocol.Unmarshal([]byte(in.Json), req)
		if err != nil {
			return nil, err
		}

		result, err := self.SignatureHelp(ctx, req)
		if err != nil {
			return nil, err
		}
		return self.returnRespose(result)

	case client.FoldingRangesOp:
		req := &protocol.FoldingRangeParams{}
		err := protocol.Unmarshal([]byte(in.Json), req)
		if err != nil {
			return nil, err
		}

		result, err := self.FoldingRanges(ctx, req)
		if err != nil {
			return nil, err
		}
		return self.returnRespose(result)

	case client.WorkspaceSymbolsOp:
		req := &protocol.WorkspaceSymbolParams{}
		err := protocol.Unmarshal([]byte(in.Json), req)
		if err != nil {
			return nil, err
		}

		result, err := self.Symbols(ctx, req)
		if err != nil {
			return nil, err
		}
		return self.returnRespose(result)

	case client.InlayHintOp:
		req := &protocol.InlayHintParams{}
		err := protocol.Unmarshal([]byte(in.Json), req)
		if err != nil {
			return nil, err
		}

		result, err := self.InlayHint(ctx, req)
		if err != nil {
			return nil, err
		}
		return self.returnRespose(result)

	case client.CodeActionOp:
		req := &protocol.CodeActionParams{}
		err := protocol.Unmarshal([]byte(in.Json), req)
		if err != nil {
			return nil, err
		}

		result, err := self.CodeAction(ctx, req)
		if err != nil {
			return nil, err
		}
		return self.returnRespose(result)

	case client.ReferencesOp:
		req := &protocol.ReferenceParams{}
		err := protocol.Unmarshal([]byte(in.Json), req)
		if err != nil {
			return nil, err
		}

		result, err := self.References(ctx, req)
		if err != nil {
			return nil, err
		}
		return self.returnRespose(result)

	case client.PrepareRenameOp:
		req := &protocol.PrepareRenameParams{}
		err := protocol.Unmarshal([]byte(in.Json), req)
		if err != nil {
			return nil, err
		}

		result, err := self.PrepareRename(ctx, req)
		if err != nil {
			return nil, err
		}
		return self.returnRespose(result)

	case client.RenameOp:
		req := &protocol.RenameParams{}
		err := protocol.Unmarshal([]byte(in.Json), req)
		if err != nil {
			return nil, err
		}

		result, err := self.Rename(ctx, req)
		if err != nil {
			return nil, err
		}
		return self.returnRespose(result)

	}
	return nil, utils.NotImplementedError
}

func NewLSPServer(config_obj *config_proto.Config) services.LSPServer {
	return &LSPServer{
		config_obj: config_obj,
		documents:  make(map[uri.URI]*Document),
	}
}
