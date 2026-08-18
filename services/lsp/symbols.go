package lsp

import (
	"context"

	"go.lsp.dev/protocol"
)

func (self *LSPServer) DocumentSymbol(
	ctx context.Context, params *protocol.DocumentSymbolParams) (
	protocol.DocumentSymbolResult, error) {

	doc, err := self.getDoc(params.TextDocument.URI)
	if err != nil {
		return nil, err
	}

	state := doc.AnalysisState
	result := []protocol.DocumentSymbol{}
	for _, definition := range state.Definitions {
		result = append(result, protocol.DocumentSymbol{
			Name:           definition.Name,
			Kind:           protocol.SymbolKindFunction,
			SelectionRange: *protocolRange(definition.Pos),
		})
	}

	for _, cs := range state.Callsites {
		cs_range := protocolRange(cs.Pos)
		cs_range.End = cs_range.Start
		cs_range.End.Character += uint32(len(cs.Name))

		result = append(result, protocol.DocumentSymbol{
			Name:           cs.Name,
			Kind:           protocol.SymbolKindFunction,
			Range:          *protocolRange(cs.Pos),
			SelectionRange: *cs_range,
		})
	}

	return protocol.DocumentSymbolSlice(result), nil
}
