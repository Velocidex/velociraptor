package lsp

import (
	"context"

	"go.lsp.dev/protocol"
	"www.velocidex.com/golang/vfilter"
	"www.velocidex.com/golang/vfilter/reformat"
)

// Formatting formats the VQL document. This is driven from the
// textDocument/formatting method.
func (self *LSPServer) Formatting(
	ctx context.Context,
	params *protocol.DocumentFormattingParams) ([]protocol.TextEdit, error) {

	doc, err := self.GetDoc(params.TextDocument.URI)
	if err != nil {
		return nil, err
	}

	formatted, err := formatVQL(doc.Text)
	if err != nil {
		// The document can not be parsed so we can not format it.
		return nil, nil
	}

	return []protocol.TextEdit{{
		Range:   *protocolRange(doc.FullRange()),
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

	return formatted, nil
}
