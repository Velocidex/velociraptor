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

	doc, err := self.GetDoc(params.TextDocument.URI)
	if err != nil {
		return nil, err
	}

	// Find the function at point
	cursor := doc.LexerPositionFromProtocol(params.Position)
	cs, err := doc.matchCallsite(cursor)

	// The position is sitting inside a call site.
	// The call site covers the function name and arg list.
	if err == nil {
		desc := doc.getVQLFunctionDescription(cs.Name, cs.Type)
		if desc == nil {
			// No hover to show.
			return &protocol.Hover{}, nil
		}

		match := doc.GetFragment(cs.Pos.Pos.Offset, cursor.Offset+1)
		if len(match) > len(cs.Name) {
			// Point is after the initial name, check maybe the point
			// is on an arg name so we can show hover about the arg.
			for _, arg := range cs.Args {
				// The arg description
				arg_desc := getArgDesc(arg.Name, desc)
				if arg_desc == nil {
					// No hover to show - the result must be null. An empty
					// Hover struct serializes contents as null which crashes
					// editor clients converting the result.
					return nil, nil
				}

				start := arg.Pos.Pos
				if start.Line == cursor.Line &&
					start.Column <= cursor.Column &&
					cursor.Column <= start.Column+len(arg.Name) {

					hover_range := protocolRange(arg.Pos)
					hover_range.End = hover_range.Start
					hover_range.End.Character += uint32(len(arg.Name))
					// Include the declared type of the argument
					// when it is known.
					arg_type := ""
					if arg_desc.Type != "" {
						arg_type = fmt.Sprintf(" (`%s`)",
							arg_desc.Type)
					}

					return &protocol.Hover{
						Range: hover_range,
						Contents: &protocol.MarkupContent{
							// MarkupContent only supports the
							// markdown and plaintext kinds.
							Kind: protocol.MarkupKindMarkdown,
							Value: fmt.Sprintf("**%s %s** arg `%s`%s: %s",
								desc.Type, desc.Name,
								arg_desc.Name, arg_type,
								arg_desc.Description),
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
				// MarkupContent only supports the markdown and
				// plaintext kinds.
				Kind: protocol.MarkupKindMarkdown,
				Value: fmt.Sprintf("**%s %s**: %s",
					desc.Type, desc.Name, desc.Description),
			},
		}, nil
	}

	// No hover to show - the result must be null. An empty
	// Hover struct serializes contents as null which crashes
	// editor clients converting the result.
	return nil, nil
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
