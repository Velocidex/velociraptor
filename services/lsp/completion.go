package lsp

import (
	"context"
	"sort"
	"strings"

	"github.com/alecthomas/participle/v2/lexer"
	"go.lsp.dev/protocol"
	api_proto "www.velocidex.com/golang/velociraptor/api/proto"
	"www.velocidex.com/golang/velociraptor/utils"
	"www.velocidex.com/golang/vfilter"
)

// Find the closest callsite around point.  This is a bit more
// complicated than it appears because calls can be nested:
//
//	e.g.
//
// foo(X=bar(z=XXXX))
// -------------^
//
// If point is at the inner callsite we want to retrieve the most
// specific callsite - in this case bar().
func (self *Document) matchCallsite(pos *lexer.Position) (callsite *vfilter.CallSite, err error) {

	type prospect_t struct {
		distance int
		cs       *vfilter.CallSite
	}

	// Find potential overlapping callsites
	var prospects []*prospect_t

	for _, cs := range self.AnalysisState.Callsites {
		cs_pos := cs.Pos

		// Potential prospect - the point is inside the callsite.
		if isPosBetween(cs_pos, pos) {
			prospects = append(prospects, &prospect_t{
				distance: pos.Offset - cs_pos.Pos.Offset,
				cs:       &cs,
			})
		}
	}

	if len(prospects) == 0 {
		return nil, utils.Wrap(
			utils.NotFoundError,
			"matchCallsite: Coordinate %v are not found in text", pos)
	}

	sort.Slice(prospects, func(i, j int) bool {
		return prospects[i].distance < prospects[j].distance
	})

	// Minimize the distance
	best_cs := prospects[0].cs
	return best_cs, nil
}

// Complete possible function names on callsite.
func (self *LSPServer) complete_function_names(
	doc *Document,
	cursor *lexer.Position,
	cs *vfilter.CallSite) (items []protocol.CompletionItem) {

	match := doc.GetFragment(cs.Pos.Pos.Offset, cursor.Offset)

	desc_range := doc.WordAtPos(*cursor, '(')
	identifier := doc.GetFragmentByRange(desc_range)

	for _, desc := range LoadApiDescriptions() {
		// A symbol can act as a plugin or a function.
		if (cs.Type == "symbol" ||
			strings.EqualFold(desc.Type, cs.Type)) &&
			strings.HasPrefix(desc.Name, match) {

			// Replace the entire identifier with the new function.
			replacement := desc.Name + "()"
			if strings.HasSuffix(identifier, "(") {
				// If the current identifier has a opening ( we assume
				// there is a closing ) somewhere ahead so we just
				// replace that.
				replacement = desc.Name + "("
			}

			short_desc := shortDescription(desc.Description)

			items = append(items, protocol.CompletionItem{
				Label: desc.Name,
				LabelDetails: &protocol.CompletionItemLabelDetails{
					Detail:      &desc.Type,
					Description: &short_desc,
				},
				Detail: protocol.NewOptional("Built in " + desc.Type),
				Kind:   getKind(desc),
				Documentation: protocol.InlayHintTooltip(
					&protocol.MarkupContent{
						Kind:  markupKind(desc.Type),
						Value: desc.Description,
					}),
				TextEdit: &protocol.TextEdit{
					Range:   *protocolRange(desc_range),
					NewText: replacement,
				},
			})
		}
	}

	return items
}

func (self *LSPServer) complete_arg_names(
	doc *Document,
	cursor *lexer.Position,
	cs *vfilter.CallSite,
) (items []protocol.CompletionItem) {

	// Find the description for the function
	desc := doc.getVQLFunctionDescription(cs.Name, cs.Type)
	if desc == nil {
		return nil
	}

	id_range := doc.WordAtPos(*cursor, '=')

	// match is the fragment between the start of the identifier and
	// the current cursor.
	match := doc.GetFragment(id_range.Pos.Offset, cursor.Offset)

	// Keep a record of existing args to the function.
	found := make(map[string]bool)
	for _, arg := range cs.Args {
		found[arg.Name] = true
	}

	// Now complete all the other args
	for _, arg_desc := range desc.Args {
		_, pres := found[arg_desc.Name]
		if pres {
			continue
		}

		item := protocol.CompletionItem{
			Label:      arg_desc.Name,
			FilterText: protocol.NewOptional(match),
			LabelDetails: &protocol.CompletionItemLabelDetails{
				Detail:      &arg_desc.Name,
				Description: &arg_desc.Description,
			},
			Detail: protocol.NewOptional(desc.Type + " arg"),
			Kind:   protocol.CompletionItemKindVariable,
			Documentation: protocol.InlayHintTooltip(
				&protocol.MarkupContent{
					Kind:  markupKind(arg_desc.Type),
					Value: arg_desc.Description,
				}),
			TextEdit: &protocol.TextEdit{
				Range:   *protocolRange(id_range),
				NewText: arg_desc.Name + "=",
			},
		}
		items = append(items, item)
	}

	return items
}

func (self *LSPServer) Completion(
	ctx context.Context,
	params *protocol.CompletionParams) ([]protocol.CompletionItem, error) {

	items := []protocol.CompletionItem{}

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
		// Make distinction between matching the callsite name itself
		// and its args
		distance_from_cs := cursor.Offset - cs.Pos.Pos.Offset

		// Match is partial name - we need to complete the name:
		// callsite: foobar(...)
		// cs_to_point match: foo
		if distance_from_cs <= len(cs.Name) {
			items = append(items, self.complete_function_names(
				doc, cursor, cs)...)
		} else {
			items = append(items, self.complete_arg_names(
				doc, cursor, cs)...)
		}
	}

	return items, nil
}

func getKind(desc *api_proto.Completion) protocol.CompletionItemKind {
	switch desc.Type {
	case "function":
		return protocol.CompletionItemKindFunction
	case "plugin":
		return protocol.CompletionItemKindMethod
	default:
		return protocol.CompletionItemKindText
	}
}

func isPosBetween(rng vfilter.RangePosition, pos *lexer.Position) bool {
	start := rng.Pos
	end := rng.EndPos

	// Line falls outside the range of interest.
	if start.Line > pos.Line || end.Line < pos.Line {
		return false
	}

	// Falls to the left of the range.
	if start.Line == pos.Line && start.Column > pos.Column {
		return false
	}

	if end.Line == pos.Line && end.Column < pos.Column {
		return false
	}

	return true
}

// Find the absolute offset in the text of the line and character
// specified.
func getNextOffset(
	text string,
	start lexer.Position,
	pos lexer.Position) (offset int, err error) {

	cur_line := start.Line
	cur_col := start.Column + 1
	for idx, char := range text[start.Offset:] {
		if char == '\n' {
			cur_line++
			cur_col = 1
		}

		if cur_col == pos.Column && cur_line == pos.Line ||
			cur_line > pos.Line {
			return idx + start.Offset, nil
		}
		cur_col++
	}
	return 0, utils.NotFoundError
}

func (self *Document) getVQLFunctionDescription(
	name, cs_type string) *api_proto.Completion {

	// Try to resolve the function from the built in set.
	desc := GetFuncDesc(name, cs_type)
	if desc != nil {
		return desc
	}

	// Maybe the descriptor is a defined function in this VQL block
	local_definition, ok := self.AnalysisState.Definitions[name]
	if !ok {
		return nil
	}

	res := &api_proto.Completion{
		Name: local_definition.Name,
		Type: local_definition.Type,
	}

	for _, arg := range local_definition.Args {
		res.Args = append(res.Args, &api_proto.ArgDescriptor{
			Name: arg.Name,
		})
	}
	return res
}

func shortDescription(desc string) string {
	const max_len = 60
	if len(desc) <= max_len {
		return desc
	}

	cut := desc[:max_len]
	idx := strings.LastIndex(cut, " ")
	if idx > max_len/2 {
		cut = cut[:idx]
	}

	return cut + " ..."
}
