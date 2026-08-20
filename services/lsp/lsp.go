package lsp

import (
	"context"
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

func (self *LSPServer) getDoc(id uri.URI) (*Document, error) {
	self.mu.Lock()
	defer self.mu.Unlock()
	doc, pres := self.documents[id]
	if !pres {
		return nil, utils.NotFoundError
	}
	return doc, nil
}

func (self *LSPServer) setDoc(id uri.URI, doc *Document) {
	self.mu.Lock()
	defer self.mu.Unlock()
	self.documents[id] = doc
}

func (self *LSPServer) returnRespose(resp any) (*api_proto.LSPResponse, error) {
	serialized, err := protocol.Marshal(resp)
	if err != nil {
		return nil, err
	}

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

	case client.DidChangeOp:
		req := &protocol.DidChangeTextDocumentParams{}
		err := protocol.Unmarshal([]byte(in.Json), req)
		if err != nil {
			return nil, err
		}

		result, err := self.DidChange(ctx, req)
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

	case client.DiagnosticOp:
		req := &protocol.DocumentDiagnosticParams{}
		err := protocol.Unmarshal([]byte(in.Json), req)
		if err != nil {
			return nil, err
		}

		result, err := self.Diagnostic(ctx, req)
		if err != nil {
			return nil, err
		}
		return self.returnRespose(result)

	case client.SymbolOp:
		req := &protocol.DocumentSymbolParams{}
		err := protocol.Unmarshal([]byte(in.Json), req)
		if err != nil {
			return nil, err
		}

		result, err := self.DocumentSymbol(ctx, req)
		if err != nil {
			return nil, err
		}
		return self.returnRespose(result)

	case client.DocumentHighlightOp:
		req := &protocol.DocumentHighlightParams{}
		err := protocol.Unmarshal([]byte(in.Json), req)
		if err != nil {
			return nil, err
		}

		result, err := self.DocumentHighlight(ctx, req)
		if err != nil {
			return nil, err
		}
		return self.returnRespose(result)

	case client.SemanticTokensOp:
		req := &protocol.SemanticTokensParams{}
		err := protocol.Unmarshal([]byte(in.Json), req)
		if err != nil {
			return nil, err
		}

		result, err := self.SemanticTokensFull(ctx, req)
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
