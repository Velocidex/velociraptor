package lsp

import (
	"context"

	"go.lsp.dev/protocol"
	"www.velocidex.com/golang/velociraptor/utils"
)

// InlayHint returns the types of the named arguments in the document.
// Since the VQL grammar requires named arguments, the client can show
// the declared type of each argument as an inlay hint after the
// argument name.
func (self *LSPServer) InlayHint(
	ctx context.Context,
	params *protocol.InlayHintParams) ([]protocol.InlayHint, error) {

	self.mu.Lock()
	doc, pres := self.documents[params.TextDocument.URI]
	self.mu.Unlock()
	if !pres {
		return nil, utils.NotFoundError
	}

	res := []protocol.InlayHint{}
	for _, cs := range doc.AnalysisState.Callsites {
		desc := doc.getVQLFunctionDescription(&cs)
		if desc == nil {
			continue
		}

		types := make(map[string]string)
		for _, arg := range desc.Args {
			if arg.Type != "" {
				types[arg.Name] = arg.Type
			}
		}

		for _, arg := range cs.Args {
			if arg.Pos.Pos.Offset <= 0 {
				continue
			}

			arg_type, pres := types[arg.Name]
			if !pres {
				continue
			}

			// The hint is placed just after the argument name.
			pos := protocolPosition(arg.Pos.Pos)
			pos.Character += uint32(len(arg.Name))

			// An empty range means the whole document.
			if params.Range != (protocol.Range{}) &&
				!rngContainsPosition(params.Range, pos) {
				continue
			}

			res = append(res, protocol.InlayHint{
				Position: pos,
				Label: protocol.InlayHintLabel(
					protocol.String("· " + arg_type)),
				Kind: protocol.InlayHintKindType,
			})
		}
	}

	return res, nil
}

// rngContainsPosition reports whether the position is inside the
// given range (inclusive of the start, exclusive of the end).
func rngContainsPosition(
	rng protocol.Range, pos protocol.Position) bool {

	if pos.Line < rng.Start.Line || pos.Line > rng.End.Line {
		return false
	}
	if pos.Line == rng.Start.Line && pos.Character < rng.Start.Character {
		return false
	}
	if pos.Line == rng.End.Line && pos.Character >= rng.End.Character {
		return false
	}
	return true
}
