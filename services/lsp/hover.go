package lsp

import (
	"context"

	"go.lsp.dev/protocol"
	"www.velocidex.com/golang/velociraptor/api/proto"
	"www.velocidex.com/golang/velociraptor/utils"
)

func (self *LSPServer) Hover(
	ctx context.Context,
	params *protocol.HoverParams) (*protocol.Hover, error) {

	self.mu.Lock()
	doc, pres := self.documents[params.TextDocument.URI]
	self.mu.Unlock()
	if !pres {
		return nil, utils.NotFoundError
	}

	// Find the function at point
	line := int(params.Position.Line)
	column := int(params.Position.Character)

	cs, offset_at_point, err := doc.matchCallsite(line, column)

	// The position is sitting inside a call site.
	// The call site covers the function name and arg list.
	if err == nil {
		desc := doc.getVQLFunctionDescription(cs)
		if desc == nil {
			return &protocol.Hover{}, nil
		}

		match := doc.Text[cs.Pos.Pos.Offset:offset_at_point]
		if len(match) < len(cs.Name) {
			// The point is on the function name - we need to display
			// hover info about the function.
			return &protocol.Hover{
				Range: &protocol.Range{
					Start: protocol.Position{
						Line:      uint32(cs.Pos.Pos.Line),
						Character: uint32(cs.Pos.Pos.Column),
					},
					End: protocol.Position{
						Line:      uint32(cs.Pos.Pos.Line),
						Character: uint32(cs.Pos.Pos.Column + len(cs.Name)),
					},
				},
				Contents: &protocol.MarkupContent{
					Kind:  protocol.MarkupKind(desc.Type),
					Value: desc.Description,
				},
			}, nil

		} else {
			// Maybe the point is on an arg name
			for _, arg := range cs.Args {
				// The arg description
				arg_desc := getArgDesc(arg.Name, desc)
				if arg_desc == nil {
					return &protocol.Hover{}, nil
				}

				if arg.Pos.Pos.Line == line &&
					arg.Pos.Pos.Column <= column &&
					column <= arg.Pos.Pos.Column+len(arg.Name) {
					return &protocol.Hover{
						Range: &protocol.Range{
							Start: protocol.Position{
								Line:      uint32(cs.Pos.Pos.Line),
								Character: uint32(cs.Pos.Pos.Column),
							},
							End: protocol.Position{
								Line:      uint32(cs.Pos.Pos.Line),
								Character: uint32(cs.Pos.Pos.Column + len(cs.Name)),
							},
						},
						Contents: &protocol.MarkupContent{
							Kind:  protocol.MarkupKind("Arg"),
							Value: arg_desc.Description,
						},
					}, nil
				}
			}
		}
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
