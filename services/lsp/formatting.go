package lsp

import (
	"context"
	"strings"

	"go.lsp.dev/protocol"
	"www.velocidex.com/golang/vfilter"
	"www.velocidex.com/golang/vfilter/reformat"
)

// Formatting formats the VQL document. This is driven from the
// textDocument/formatting method.
func (self *LSPServer) Formatting(
	ctx context.Context,
	params *protocol.DocumentFormattingParams) ([]protocol.TextEdit, error) {

	self.mu.Lock()
	doc, pres := self.documents[params.TextDocument.URI]
	self.mu.Unlock()
	if !pres {
		return nil, nil
	}

	formatted, err := formatVQL(doc.Text)
	if err != nil {
		// The document can not be parsed so we can not format it.
		return nil, nil
	}

	return []protocol.TextEdit{{
		Range:   fullDocumentRange(doc.Text),
		NewText: formatted,
	}}, nil
}

// formatVQL reformats the query in place. Returns an empty string if
// the query can not be parsed.
func formatVQL(query string) (string, error) {
	formatted, err := reformat.ReFormatVQL(
		vfilter.NewScope(), query, vfilter.DefaultFormatOptions)
	if err != nil {
		return "", err
	}

	return strings.TrimRight(formatted, " \n"), nil
}

// fullDocumentRange returns a range covering the entire document.
func fullDocumentRange(document string) protocol.Range {
	lines := strings.Split(document, "\n")
	last_line := len(lines) - 1
	last_char := len(lines[last_line])

	return protocol.Range{
		Start: protocol.Position{Line: 0, Character: 0},
		End: protocol.Position{
			Line:      uint32(last_line),
			Character: uint32(last_char),
		},
	}
}
