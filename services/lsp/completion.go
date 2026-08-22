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
			items = append(items,
				pluginFunctionCompletionItem(desc))
		}
	}

	return items
}

// shortDescription caps a description to roughly one popup row so
// editors do not need to truncate it with an ellipsis. The full text
// remains available in the item documentation.
func shortDescription(desc string) string {
	const max_len = 60
	if len(desc) <= max_len {
		return desc
	}

	cut := desc[:max_len]
	if idx := strings.LastIndex(cut, " "); idx > max_len/2 {
		cut = cut[:idx]
	}
	return cut
}

// pluginFunctionCompletionItem builds the completion item for a built
// in plugin or function. The label detail carries the type with a
// leading space because editors render it directly appended to the
// label.
//
// FilterText carries a trailing '.' so editors keep matching the item
// when the trigger character is part of the replaced range: editors
// build their filter query from the characters an item overwrites
// (e.g. "pars.") and a plain label like "parse_auditd" fails that
// fuzzy match.
func pluginFunctionCompletionItem(
	desc *api_proto.Completion) protocol.CompletionItem {

	label_detail := " " + desc.Type
	description := shortDescription(desc.Description)
	filter_text := desc.Name + "."

	return protocol.CompletionItem{
		Label:      desc.Name,
		FilterText: protocol.NewOptional(filter_text),
		LabelDetails: &protocol.CompletionItemLabelDetails{
			Detail:      &label_detail,
			Description: &description,
		},
		Detail: protocol.NewOptional("Built in " + desc.Type),
		Documentation: protocol.InlayHintTooltip(&protocol.MarkupContent{
			Kind:  markupKind(desc.Type),
			Value: desc.Description,
		}),
		Kind: getKind(desc),
	}
}

// afterFromKeyword returns true when the word ending at word_start is
// preceded only by a FROM keyword, i.e. the cursor is in plugin
// position where functions are not valid.
func afterFromKeyword(text string, word_start int) bool {
	i := word_start

	// Skip whitespace between the keyword and the word.
	for i > 0 {
		c := text[i-1]
		if c == ' ' || c == '\t' || c == '\n' || c == '\r' {
			i--
			continue
		}
		break
	}

	// Walk back over the word itself (which may be dotted).
	end := i
	for i > 0 {
		c := text[i-1]
		if c == '.' || c == '_' || (c >= 'a' && c <= 'z') ||
			(c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9') {
			i--
			continue
		}
		break
	}

	return strings.EqualFold(text[i:end], "from")
}

// complete_prefix_names suggests plugins, functions and LET variables
// matching the given prefix. This is the fallback used when the cursor
// is not inside a call site - e.g. "SELECT * FROM parse." should
// suggest all the parse* plugins.
//
// edit_range covers the typed prefix in the document (including any
// trailing '.' trigger character) so the client replaces it with the
// full name instead of appending to it.
//
// When plugin_position is set the cursor directly follows FROM and
// only plugins are offered - functions are not valid there.
func (self *LSPServer) complete_prefix_names(
	doc *Document, match string,
	edit_range *protocol.Range,
	plugin_position bool) (items []protocol.CompletionItem) {

	// LET variables defined in the document.
	for name := range doc.AnalysisState.Definitions {
		if strings.HasPrefix(name, match) {
			items = append(items, protocol.CompletionItem{
				Label:  name,
				Kind:   protocol.CompletionItemKindVariable,
				Detail: protocol.NewOptional("LET variable"),
			})
			if edit_range != nil {
				items[len(items)-1].TextEdit = protocol.CompletionItemTextEdit(
					&protocol.TextEdit{
						Range:   *edit_range,
						NewText: name,
					})
			}
		}
	}

	// Offset just past the replaced range to detect an already
	// present open paren.
	next_offset := -1
	if edit_range != nil {
		next_offset = positionToOffset(doc.Text, edit_range.End)
	}

	// Built in plugins and functions.
	for _, desc := range LoadApiDescriptions() {
		if !strings.HasPrefix(desc.Name, match) {
			continue
		}

		if plugin_position &&
			strings.EqualFold(desc.Type, "function") {
			continue
		}

		item := pluginFunctionCompletionItem(desc)

		// Leave the cursor between an inserted paren pair unless
		// one is already present right after the name.
		new_text := desc.Name
		if next_offset < 0 || next_offset >= len(doc.Text) ||
			doc.Text[next_offset] != '(' {
			new_text += "($0)"
			item.InsertTextFormat = protocol.InsertTextFormatSnippet
		}

		if edit_range != nil {
			item.TextEdit = protocol.CompletionItemTextEdit(
				&protocol.TextEdit{
					Range:   *edit_range,
					NewText: new_text,
				})
		}
		items = append(items, item)
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

		// A cursor at the end of a partially typed name
		// (distance == len) is included so completing right
		// after typing "fil" suggests "filename".
		if distance_to_start_of_arg > 0 &&
			distance_to_start_of_arg <= len(arg.Name) {
			match := arg.Name[:distance_to_start_of_arg]

			for _, arg_desc := range desc.Args {
				if strings.HasPrefix(arg_desc.Name, match) {
					items = append(items,
						argCompletionItem(desc, arg_desc))
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

			items = append(items, argCompletionItem(desc, arg_desc))
		}
		return items
	}

	return items
}

// argCompletionItem builds a completion for a named argument. The
// declared type is surfaced in the label details and documentation so
// editors can display it in the popup and agents can read it from the
// structured response.
func argCompletionItem(
	desc *api_proto.Completion,
	arg_desc *api_proto.ArgDescriptor) protocol.CompletionItem {

	label_detail := ""
	if arg_desc.Type != "" {
		label_detail = ": " + arg_desc.Type
	}

	documentation := arg_desc.Description
	if arg_desc.Type != "" {
		documentation = "Type: `" + arg_desc.Type + "`\n\n" +
			arg_desc.Description
	}

	description := shortDescription(arg_desc.Description)

	return protocol.CompletionItem{
		Label: arg_desc.Name,
		LabelDetails: &protocol.CompletionItemLabelDetails{
			Detail:      &label_detail,
			Description: &description,
		},
		Detail: protocol.NewOptional(desc.Name + " arg"),
		Documentation: protocol.InlayHintTooltip(&protocol.MarkupContent{
			Kind:  protocol.MarkupKindMarkdown,
			Value: documentation,
		}),
		Kind: protocol.CompletionItemKindVariable,
	}
}

// findFunctionBeforeParen looks for a function name immediately before
// an open paren at the given position. Returns the function name and
// its start offset if found.
func (self *Document) findFunctionBeforeParen(pos lexer.Position) (string, int, error) {
	protoPos := protocol.Position{
		Line:      uint32(pos.Line - 1),
		Character: uint32(pos.Column - 1),
	}
	offset := positionToOffset(self.Text, protoPos)
	if offset <= 0 {
		return "", 0, utils.NotFoundError
	}

	// Walk backwards to find the opening paren
	parenOffset := -1
	for i := offset - 1; i >= 0; i-- {
		if self.Text[i] == '(' {
			parenOffset = i
			break
		}
		if self.Text[i] == '\n' {
			break // Don't cross line boundaries
		}
	}
	if parenOffset == -1 {
		return "", 0, utils.NotFoundError
	}

	// Walk backwards from paren to find the function name
	end := parenOffset
	for end > 0 && (isIdentChar(self.Text[end-1]) || self.Text[end-1] == '.') {
		end--
	}
	if end == parenOffset {
		return "", 0, utils.NotFoundError
	}

	return self.Text[end:parenOffset], end, nil
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

		// Plain symbols (bare identifiers) have no descriptions -
		// fall through to the prefix based completion instead of
		// returning nothing.
		if len(items) > 0 || !strings.EqualFold(cs.Type, "symbol") {
			return items, nil
		}
	}

	// The cursor is not inside a call site - e.g. the user typed
	// "SELECT * FROM parse." and the '.' trigger fired completion.
	// But first, check if we are right after an open paren - if so,
	// provide argument completions for the function before the paren.
	// This covers queries that do not parse yet, such as an unclosed
	// call like "SELECT * FROM pslist(".
	offset := positionToOffset(doc.Text, params.Position)
	if offset > 0 && doc.Text[offset-1] == '(' {
		if name, _, err := doc.findFunctionBeforeParen(pos); err == nil {
			// Find the function description
			for _, desc := range LoadApiDescriptions() {
				if strings.EqualFold(desc.Name, name) {
					// Create a CallSite with no provided args so
					// complete_arg_names offers every unused one.
					cs := &vfilter.CallSite{
						Name: name,
						Type: desc.Type,
						Args: []vfilter.ArgDesc{},
					}
					items = append(items, self.complete_arg_names(
						doc, cs, offset)...)
					break
				}
			}
			if len(items) > 0 {
				return items, nil
			}
		}
	}

	// Fall back to prefix based completion of plugins, functions and
	// LET variables.
	offset = positionToOffset(doc.Text, params.Position)
	prefix := wordPrefix(doc.Text, offset)

	// The edit range covers the typed prefix including any trailing
	// '.' trigger character, so the client replaces it with the full
	// name rather than appending to it.
	edit_range := protocol.Range{
		Start: offsetToPosition(doc.Text, offset-len(prefix)),
		End:   offsetToPosition(doc.Text, offset),
	}

	// A trailing '.' (the trigger character) should not prevent
	// matching the callable name itself.
	plugin_position := afterFromKeyword(doc.Text, offset-len(prefix))
	prefix = strings.TrimSuffix(prefix, ".")

	return self.complete_prefix_names(doc, prefix, &edit_range,
		plugin_position), nil
}

// wordPrefix returns the identifier prefix ending at the given byte
// offset. Identifiers in VQL can contain letters, digits, underscores
// and dots (for dotted plugin names like Artifact.Linux.Sys.Users).
func wordPrefix(text string, offset int) string {
	// The client may report a position beyond the end of the
	// document (e.g. its view of the text is briefly ahead of the
	// server) - clamp it instead of indexing out of range.
	if offset > len(text) {
		offset = len(text)
	}
	if offset < 0 {
		offset = 0
	}

	start := offset
	for start > 0 {
		c := text[start-1]
		if c == '.' || c == '_' || (c >= 'a' && c <= 'z') ||
			(c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9') {
			start--
			continue
		}
		break
	}
	return text[start:offset]
}

// positionToOffset converts a 0 based LSP position to a byte offset in
// the document. Positions beyond the end of the document (possible
// when the client's view of the text is ahead of the server) are
// clamped to the end of the document.
func positionToOffset(text string, pos protocol.Position) int {
	line := 0
	offset := 0
	for offset < len(text) && line < int(pos.Line) {
		if text[offset] == '\n' {
			line++
		}
		offset++
	}
	offset += int(pos.Character)
	if offset > len(text) {
		offset = len(text)
	}
	return offset
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
