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
func (self *Document) matchCallsite(pos lexer.Position) (
	callsite *vfilter.CallSite, offset_at_point int, err error) {

	type prospect_t struct {
		distance int
		cs       *vfilter.CallSite
	}

	// Find potential overlapping callsites
	var prospects []*prospect_t

	offset_at_point = -1

	for _, cs := range self.AnalysisState.Callsites {
		cs_pos := cs.Pos

		// Potential prospect - the point is inside the callsite.
		if isPosBetween(cs_pos, pos) {
			if offset_at_point < 0 {
				offset_at_point, err = getNextOffset(
					self.Text, cs_pos.Pos, pos)
				if err != nil {
					// If we can not get the exact offset, then the
					// text does not match the offset.
					break
				}
			}

			prospects = append(prospects, &prospect_t{
				distance: offset_at_point - cs_pos.Pos.Offset,
				cs:       &cs,
			})
		}
	}

	if len(prospects) == 0 {
		return nil, 0, utils.Wrap(
			utils.NotFoundError,
			"matchCallsite: Coordinate %v are not found in text", pos)
	}

	sort.Slice(prospects, func(i, j int) bool {
		return prospects[i].distance < prospects[j].distance
	})

	// Minimize the distance
	best_cs := prospects[0].cs
	return best_cs, offset_at_point, nil
}

// Complete possible function names on callsite.
func (self *LSPServer) complete_function_names(
	cs *vfilter.CallSite,
	match string) (items []protocol.CompletionItem) {
	for _, desc := range LoadApiDescriptions() {
		if strings.EqualFold(desc.Type, cs.Type) &&
			strings.HasPrefix(desc.Name, match) {
			items = append(items, protocol.CompletionItem{
				Label: desc.Name,
				LabelDetails: &protocol.CompletionItemLabelDetails{
					Detail:      &desc.Type,
					Description: &desc.Description,
				},
				Detail: protocol.NewOptional("Built in " + desc.Type),
				Kind:   getKind(desc),
			})
		}
	}

	return items
}

func (self *LSPServer) complete_arg_names(
	doc *Document,
	cs *vfilter.CallSite,
	offset_at_point int) (items []protocol.CompletionItem) {

	// Find the description for the function
	desc := doc.getVQLFunctionDescription(cs)
	if desc == nil {
		return nil
	}

	found := make(map[string]bool)

	// Determine the arg that is on point
	for _, arg := range cs.Args {
		found[arg.Name] = true
		distance_to_start_of_arg := offset_at_point - arg.Pos.Pos.Offset

		if distance_to_start_of_arg > 0 &&
			distance_to_start_of_arg < len(arg.Name) {
			match := arg.Name[:distance_to_start_of_arg]

			// Point is past the arg name - no completion available.
			if len(match) > len(arg.Name) {
				return items
			}

			for _, arg_desc := range desc.Args {
				if strings.HasPrefix(arg_desc.Name, match) {
					items = append(items, protocol.CompletionItem{
						Label: arg_desc.Name,
						LabelDetails: &protocol.CompletionItemLabelDetails{
							Detail:      &arg_desc.Name,
							Description: &arg_desc.Description,
						},
						Detail: protocol.NewOptional(desc.Type + " arg"),
						Kind:   protocol.CompletionItemKindVariable,
					})
				}
			}
		}
	}

	if len(items) == 0 {
		for _, arg_desc := range desc.Args {
			_, pres := found[arg_desc.Name]
			if pres {
				continue
			}

			items = append(items, protocol.CompletionItem{
				Label: arg_desc.Name,
				LabelDetails: &protocol.CompletionItemLabelDetails{
					Detail:      &arg_desc.Name,
					Description: &arg_desc.Description,
				},
				Detail: protocol.NewOptional(desc.Type + " arg"),
				Kind:   protocol.CompletionItemKindVariable,
			})
		}
		return items
	}

	return items
}

func (self *LSPServer) Completion(
	ctx context.Context,
	params *protocol.CompletionParams) ([]protocol.CompletionItem, error) {

	items := []protocol.CompletionItem{}

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
		match := doc.getFragment(cs.Pos.Pos.Offset, offset_at_point)

		// Match is partial name - we need to complete the name:
		// callsite: foobar(...)
		// cs_to_point match: foo
		if len(match) < len(cs.Name) {
			items = append(items, self.complete_function_names(
				cs, match)...)
		} else {
			items = append(items, self.complete_arg_names(
				doc, cs, offset_at_point)...)
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

func isPosBetween(rng vfilter.RangePosition, pos lexer.Position) bool {
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
	cur_col := start.Column
	for idx, char := range text[start.Offset:] {
		if cur_col == pos.Column && cur_line == pos.Line {
			return idx + start.Offset, nil
		}

		if char == '\n' {
			cur_line++
			cur_col = 1
		} else {
			cur_col++
		}

		if cur_line > pos.Line {
			break
		}
	}
	return 0, utils.NotFoundError
}

func (self *Document) getVQLFunctionDescription(
	cs *vfilter.CallSite) *api_proto.Completion {

	for _, desc := range LoadApiDescriptions() {
		if desc.Name == cs.Name &&
			strings.EqualFold(desc.Type, cs.Type) {
			return desc
		}
	}

	// Maybe the descriptor is a defined function
	local_definition, ok := self.AnalysisState.Definitions[cs.Name]
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
