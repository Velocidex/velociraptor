package lsp

import (
	"context"

	"go.lsp.dev/protocol"
	"www.velocidex.com/golang/velociraptor/utils"
	"www.velocidex.com/golang/vfilter"
)

// FoldingRanges returns the regions of the document which can be
// folded by the editor. We fold every top level query and LET
// definition that spans multiple lines.
func (self *LSPServer) FoldingRanges(
	ctx context.Context,
	params *protocol.FoldingRangeParams) ([]protocol.FoldingRange, error) {

	self.mu.Lock()
	doc, pres := self.documents[params.TextDocument.URI]
	self.mu.Unlock()
	if !pres {
		return nil, utils.NotFoundError
	}

	// Collect the spans of all top level queries and definitions.
	// The LET definition is included in the span of the query
	// statement itself so we deduplicate identical spans.
	seen := make(map[vfilter.RangePosition]bool)
	spans := []vfilter.RangePosition{}
	for _, q := range doc.AnalysisState.TopLevelQueries {
		if !seen[q.Pos] {
			seen[q.Pos] = true
			spans = append(spans, q.Pos)
		}
	}
	for _, def := range doc.AnalysisState.Definitions {
		if !seen[def.Pos] {
			seen[def.Pos] = true
			spans = append(spans, def.Pos)
		}
	}

	res := []protocol.FoldingRange{}
	for _, span := range spans {
		start := protocolPosition(span.Pos)
		end := protocolPosition(span.EndPos)

		// Only fold spans that cover more than one line.
		if end.Line > start.Line {
			res = append(res, protocol.FoldingRange{
				StartLine: start.Line,
				EndLine:   end.Line,
				Kind:      protocol.FoldingRangeKindRegion,
			})
		}
	}

	return res, nil
}
