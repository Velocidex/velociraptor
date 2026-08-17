package lsp

import (
	"sort"

	"github.com/alecthomas/participle/v2/lexer"
	"go.lsp.dev/protocol"
	"www.velocidex.com/golang/vfilter"
)

// positionMapper converts between byte offsets (as reported by the
// participle lexer) and the 0 based line/column positions used by the
// LSP protocol.
//
// participle uses 1 based line and column counters internally so we
// never use them directly. Instead we convert only using the byte
// offsets via the line start table of the document. Byte offsets are
// unambiguous - they only depend on the document text itself.
type positionMapper struct {
	document    string
	line_starts []int
}

func newPositionMapper(document string) *positionMapper {
	res := &positionMapper{
		document:    document,
		line_starts: []int{0},
	}

	for idx, char := range document {
		if char == '\n' {
			res.line_starts = append(res.line_starts, idx+1)
		}
	}

	return res
}

// mapPos converts a byte offset to a 0 based LSP position.
func (self *positionMapper) mapOffset(offset int) protocol.Position {
	if offset < 0 {
		offset = 0
	}
	if offset > len(self.document) {
		offset = len(self.document)
	}

	// Binary search for the line containing the offset.
	line := sort.Search(len(self.line_starts), func(i int) bool {
		return self.line_starts[i] > offset
	}) - 1
	if line < 0 {
		line = 0
	}

	return protocol.Position{
		Line:      uint32(line),
		Character: uint32(offset - self.line_starts[line]),
	}
}

// mapPos converts a participle position to a 0 based LSP position.
func (self *positionMapper) mapPos(pos lexer.Position) protocol.Position {
	return self.mapOffset(pos.Offset)
}

// mapRange converts a participle RangePosition to an LSP range.
func (self *positionMapper) mapRange(rng vfilter.RangePosition) protocol.Range {
	return protocol.Range{
		Start: self.mapPos(rng.Pos),
		End:   self.mapPos(rng.EndPos),
	}
}

// positionToOffset converts a 0 based LSP position to a byte offset
// in the document. Positions past the end of the document are clamped.
func (self *positionMapper) positionToOffset(
	line, column int) int {
	if line < 0 || line >= len(self.line_starts) {
		return len(self.document)
	}

	offset := self.line_starts[line] + column
	if offset < 0 {
		return 0
	}
	if offset > len(self.document) {
		return len(self.document)
	}
	return offset
}

// rngContains reports whether the given byte offset is inside the
// range (inclusive of both ends).
func rngContains(rng vfilter.RangePosition, offset int) bool {
	return offset >= rng.Pos.Offset && offset <= rng.EndPos.Offset
}
