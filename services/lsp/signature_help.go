package lsp

import (
	"context"
	"strings"

	"go.lsp.dev/protocol"
	api_proto "www.velocidex.com/golang/velociraptor/api/proto"
	"www.velocidex.com/golang/velociraptor/utils"
	"www.velocidex.com/golang/vfilter"
)

// SignatureHelp provides the signature of the call under the cursor.
func (self *LSPServer) SignatureHelp(
	ctx context.Context,
	params *protocol.SignatureHelpParams) (*protocol.SignatureHelp, error) {

	self.mu.Lock()
	doc, pres := self.documents[params.TextDocument.URI]
	self.mu.Unlock()
	if !pres {
		return nil, utils.NotFoundError
	}

	pos := lexerPositionFromProtocol(params.Position)
	cs, offset_at_point, err := doc.matchCallsite(pos)
	if err != nil {
		return nil, nil
	}

	desc := doc.getVQLFunctionDescription(cs)
	if desc == nil {
		return nil, nil
	}

	return buildSignatureHelp(desc, cs, offset_at_point)
}

// buildSignatureHelp renders the description as a signature and
// selects the active parameter from the callsite argument positions.
func buildSignatureHelp(
	desc *api_proto.Completion,
	cs *vfilter.CallSite,
	offset_at_point int) (*protocol.SignatureHelp, error) {

	signature := protocol.SignatureInformation{
		Label: buildSignatureLabel(desc),
		Documentation: protocol.InlayHintTooltip(&protocol.MarkupContent{
			Kind:  markupKind(desc.Type),
			Value: desc.Description,
		}),
		Parameters: []protocol.ParameterInformation{},
	}

	active_parameter := 0
	for _, arg := range desc.Args {
		param := protocol.ParameterInformation{
			Label: protocol.String(arg.Name + ": " + arg.Type),
		}
		if arg.Description != "" {
			param.Documentation = protocol.InlayHintTooltip(
				&protocol.MarkupContent{
					Kind:  protocol.MarkupKindPlainText,
					Value: arg.Description,
				})
		}
		signature.Parameters = append(signature.Parameters, param)
	}

	// The VQL grammar requires named arguments so we can use the
	// callsite args to determine which parameter is active. Use the
	// declared-argument index of the last argument started before the
	// cursor, otherwise fall back to the next free slot.
	active_name := ""
	for _, arg := range cs.Args {
		if arg.Pos.Pos.Offset > 0 && arg.Pos.Pos.Offset > offset_at_point {
			break
		}
		active_name = arg.Name
	}
	if active_name != "" {
		for idx, declared := range desc.Args {
			if declared.Name == active_name {
				active_parameter = idx
				break
			}
		}
	} else if len(cs.Args) < len(signature.Parameters) {
		active_parameter = len(cs.Args)
	}

	active := uint32(active_parameter)
	return &protocol.SignatureHelp{
		Signatures:      []protocol.SignatureInformation{signature},
		ActiveSignature: &active,
		ActiveParameter: protocol.NewNullable(active),
	}, nil
}

func buildSignatureLabel(desc *api_proto.Completion) string {
	args := []string{}
	for _, arg := range desc.Args {
		if arg.Type != "" {
			args = append(args, arg.Name+": "+arg.Type)
		} else {
			args = append(args, arg.Name)
		}
	}
	return desc.Name + "(" + strings.Join(args, ", ") + ")"
}

func markupKind(desc_type string) protocol.MarkupKind {
	switch strings.ToLower(desc_type) {
	case "plugin", "function":
		return protocol.MarkupKindMarkdown
	default:
		return protocol.MarkupKindPlainText
	}
}
