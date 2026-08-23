package lsp

import (
	"context"
	"sort"

	"go.lsp.dev/protocol"
)

// DocumentSymbol implements textDocument/documentSymbol. It returns
// the LET variables defined in the requested document as an outline
// of the document.
func (self *LSPServer) DocumentSymbol(
	ctx context.Context,
	params *protocol.DocumentSymbolParams) (protocol.DocumentSymbolResult, error) {

	res := []protocol.DocumentSymbol{}

	self.mu.Lock()
	defer self.mu.Unlock()

	doc, pres := self.documents[params.TextDocument.URI]
	if !pres {
		// Unknown document - return an empty outline.
		return protocol.DocumentSymbolSlice(res), nil
	}

	for _, def := range doc.AnalysisState.Definitions {
		symbol := protocol.DocumentSymbol{
			Name:           def.Name,
			Kind:           protocol.SymbolKindVariable,
			Range:          *protocolRange(def.Pos),
			SelectionRange: *protocolRange(def.Pos),
		}

		// Functions are declared with LET Foo(X, Y) = ...
		if len(def.Args) > 0 {
			symbol.Kind = protocol.SymbolKindFunction
		}

		res = append(res, symbol)
	}

	sort.Slice(res, func(i, j int) bool {
		return res[i].Name < res[j].Name
	})

	return protocol.DocumentSymbolSlice(res), nil
}
