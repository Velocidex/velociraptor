package lsp

import (
	"context"

	"go.lsp.dev/protocol"
)

func (self *LSPServer) DocumentHighlight(
	ctx context.Context, params *protocol.DocumentHighlightParams) (
	res []protocol.DocumentHighlight, err error) {

	return res, nil
}
