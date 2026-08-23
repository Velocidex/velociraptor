package lsp

import (
	"bytes"
	"encoding/json"

	"github.com/alecthomas/participle/v2/lexer"
	"go.lsp.dev/protocol"
	"www.velocidex.com/golang/vfilter"
)

func (self *Document) getFragment(start, end int) string {
	if start < 0 {
		start = 0
	}

	if end < 0 {
		end = 0
	}

	if end < start {
		return ""
	}

	if end > len(self.Text) {
		end = len(self.Text)
	}

	return self.Text[start:end]
}

func DumpProtool(in interface{}) string {
	serialized, err := protocol.Marshal(in)
	if err != nil {
		return ""
	}
	dst := &bytes.Buffer{}
	err = json.Indent(dst, serialized, "", " ")
	if err != nil {
		return ""
	}
	return dst.String()
}

// Convert vfilter ranges to lsp protocol ranges.  vfilter Ranges are
// 1 based counter (i.e. line numbers start at 1), while LSP counters
// start at 0.
func protocolRange(in vfilter.RangePosition) *protocol.Range {
	return &protocol.Range{
		Start: protocolPosition(in.Pos),
		End:   protocolPosition(in.EndPos),
	}
}

func protocolPosition(in lexer.Position) protocol.Position {
	// Lexer positions start with 1 so a 0 means an error.
	if in.Line == 0 || in.Column == 0 {
		return protocol.Position{}
	}

	return protocol.Position{
		Line:      uint32(in.Line) - 1,
		Character: uint32(in.Column) - 1,
	}
}

// Convert from 0 based lsp protocol positions to 1 based lexer
// positions.
func lexerPositionFromProtocol(in protocol.Position) lexer.Position {
	return lexer.Position{
		Line:   int(in.Line + 1),
		Column: int(in.Character + 1),
	}
}

// offsetToPosition converts a byte offset in the document to a 0
// based LSP position. It is used by the features which work on raw
// byte offsets (references, rename) to convert the offsets they
// compute from the document text.
func offsetToPosition(text string, offset int) protocol.Position {
	if offset < 0 {
		offset = 0
	}
	if offset > len(text) {
		offset = len(text)
	}

	line := 0
	for idx, char := range text {
		if idx >= offset {
			break
		}
		if char == '\n' {
			line++
		}
	}

	// Find the start of the line containing the offset.
	line_start := 0
	for idx, char := range text {
		if idx >= offset {
			break
		}
		if char == '\n' {
			line_start = idx + 1
		}
	}

	return protocol.Position{
		Line:      uint32(line),
		Character: uint32(offset - line_start),
	}
}
