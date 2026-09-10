package lsp

import (
	"context"
	"sort"
	"strings"

	"go.lsp.dev/protocol"
)

// Symbols implements workspace/symbol. The workspace consists of the
// open documents, so we return the LET variables defined in them
// together with the built in plugins and functions.
func (self *LSPServer) Symbols(
	ctx context.Context,
	params *protocol.WorkspaceSymbolParams) (protocol.WorkspaceSymbolResult, error) {

	res := []protocol.WorkspaceSymbol{}

	query := strings.ToLower(params.Query)

	self.mu.Lock()
	for uri, doc := range self.documents {
		for _, def := range doc.AnalysisState.Definitions {
			if query != "" &&
				!strings.Contains(strings.ToLower(def.Name), query) {
				continue
			}

			symbol := protocol.WorkspaceSymbol{
				Location: &protocol.Location{
					URI: uri,
					Range: protocol.Range{
						Start: protocolPosition(def.Pos.Pos),
						End:   protocolPosition(def.Pos.EndPos),
					},
				},
			}
			symbol.Name = def.Name
			symbol.Kind = protocol.SymbolKindVariable
			res = append(res, symbol)
		}
	}
	self.mu.Unlock()

	// Also include the built in plugins and functions. These do not
	// have a location in the workspace.
	for _, desc := range LoadApiDescriptions() {
		if query != "" &&
			!strings.Contains(strings.ToLower(desc.Name), query) {
			continue
		}

		symbol := protocol.WorkspaceSymbol{
			Location: nil,
		}
		symbol.Name = desc.Name
		if strings.EqualFold(desc.Type, "function") {
			symbol.Kind = protocol.SymbolKindFunction
		} else {
			symbol.Kind = protocol.SymbolKindClass
		}
		res = append(res, symbol)
	}

	sort.Slice(res, func(i, j int) bool {
		return res[i].Name < res[j].Name
	})

	return protocol.WorkspaceSymbolSlice(res), nil
}
