package lsp

import (
	"context"
	"fmt"

	"go.lsp.dev/protocol"
	"www.velocidex.com/golang/velociraptor/api/proto"
)

func (self *LSPServer) Hover(
	ctx context.Context,
	params *protocol.HoverParams) (*protocol.Hover, error) {

	doc, err := self.getDoc(params.TextDocument.URI)
	if err != nil {
		return nil, err
	}

	// Find the function at point
	pos := lexerPositionFromProtocol(params.Position)
	cs, offset_at_point, err := doc.matchCallsite(pos)

	// The position is sitting inside a call site.
	// The call site covers the function name and arg list.
	if err == nil {
		desc := doc.getVQLFunctionDescription(cs)
		if desc == nil {
			return &protocol.Hover{}, nil
		}

		match := doc.getFragment(cs.Pos.Pos.Offset, offset_at_point)
		if len(match) > len(cs.Name) {
			// Point is after the initial name, check maybe the point
			// is on an arg name so we can show hover about the arg.
			for _, arg := range cs.Args {
				// The arg description
				arg_desc := getArgDesc(arg.Name, desc)
				if arg_desc == nil {
					return &protocol.Hover{}, nil
				}

				start := arg.Pos.Pos
				if start.Line == pos.Line &&
					start.Column <= pos.Column &&
					pos.Column <= start.Column+len(arg.Name) {

					hover_range := protocolRange(arg.Pos)
					hover_range.End = hover_range.Start
					hover_range.End.Character += uint32(len(arg.Name))
					return &protocol.Hover{
						Range: hover_range,
						Contents: &protocol.MarkupContent{
							Kind: protocol.MarkupKind("Arg"),
							Value: fmt.Sprintf("%s %s: arg %s: %s",
								desc.Type, desc.Name,
								arg_desc.Name, arg_desc.Description),
						},
					}, nil
				}
			}
		}

		// The point is on the function name or somewhere else within
		// the function args. - we need to display hover info about
		// the function. Only highlight the function name instead of
		// all of it.
		hover_range := protocolRange(cs.Pos)
		hover_range.End = hover_range.Start
		hover_range.End.Character += uint32(len(cs.Name))

		return &protocol.Hover{
			Range: hover_range,
			Contents: &protocol.MarkupContent{
				Kind: protocol.MarkupKind(desc.Type),
				Value: fmt.Sprintf("%s %s: %s",
					desc.Type, desc.Name, desc.Description),
			},
		}, nil
	}

	return &protocol.Hover{}, nil
}

func getArgDesc(arg_name string,
	desc *proto.Completion) *proto.ArgDescriptor {
	for _, arg := range desc.Args {
		if arg.Name == arg_name {
			return arg
		}
	}
	return nil
}
