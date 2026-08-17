package lsp

import (
	"context"
	"strings"
	"unicode"

	"go.lsp.dev/protocol"
	"www.velocidex.com/golang/velociraptor/utils"
	vql_subsystem "www.velocidex.com/golang/velociraptor/vql"
	"www.velocidex.com/golang/vfilter"
)

// References returns all the places in the document where the
// identifier under the cursor is used. Currently this resolves LET
// variables: the definition (optionally) and every bare symbol
// reference to it.
func (self *LSPServer) References(
	ctx context.Context,
	params *protocol.ReferenceParams) ([]protocol.Location, error) {

	self.mu.Lock()
	doc, pres := self.documents[params.TextDocument.URI]
	self.mu.Unlock()
	if !pres {
		return nil, utils.NotFoundError
	}

	position_mapper := newPositionMapper(doc.Text)
	offset_at_point := position_mapper.positionToOffset(
		int(params.Position.Line), int(params.Position.Character))

	name := wordAtOffset(doc.Text, offset_at_point)
	if name == "" {
		return nil, nil
	}

	// The analysis state cached in the document only keeps the
	// callsites of the last statement, so reparse the whole document
	// to collect every definition and callsite.
	definitions, callsites, err := analyseDocument(doc.Text)
	if err != nil {
		return nil, err
	}

	// Registry plugins and functions are global so there is no useful
	// definition to resolve in the document.
	def, pres := definitions[name]
	if !pres {
		return nil, nil
	}

	res := []protocol.Location{}
	seen := make(map[protocol.Range]bool)

	addLocation := func(offset int, length int) {
		if offset < 0 || offset+length > len(doc.Text) {
			return
		}
		rng := protocol.Range{
			Start: position_mapper.mapOffset(offset),
			End:   position_mapper.mapOffset(offset + length),
		}
		if seen[rng] {
			return
		}
		seen[rng] = true
		res = append(res, protocol.Location{
			URI:   params.TextDocument.URI,
			Range: rng,
		})
	}

	if params.Context.IncludeDeclaration {
		addLocation(definitionNameOffset(doc.Text, def), len(name))
	}

	for _, cs := range callsites {
		if cs.Type == "symbol" && cs.Name == name {
			addLocation(cs.Pos.Pos.Offset, len(cs.Name))
		}
	}

	return res, nil
}

// analyseDocument parses the whole document and collects every
// definition and callsite, across all statements.
func analyseDocument(query string) (
	map[string]vfilter.DefinitionSite, []vfilter.CallSite, error) {

	scope := vql_subsystem.MakeScope()

	vqls, err := vfilter.MultiParse(query)
	if err != nil {
		return nil, nil, err
	}

	definitions := make(map[string]vfilter.DefinitionSite)
	var callsites []vfilter.CallSite

	for _, vql := range vqls {
		visitor := vfilter.NewVisitor(
			scope, vfilter.CollectAllInfo)
		visitor.Visit(vql)

		for _, def := range visitor.Definitions {
			definitions[def.Name] = def
		}
		callsites = append(callsites, visitor.CallSites...)
	}

	return definitions, callsites, nil
}

// wordAtOffset returns the identifier spanning the given byte offset.
// Identifiers may include dots for qualified names like
// Artifact.Linux.Sys.Users.
func wordAtOffset(text string, offset int) string {
	if offset >= len(text) {
		offset = len(text) - 1
	}
	if offset < 0 {
		return ""
	}

	start := offset
	for start > 0 && isIdentChar(text[start-1]) {
		start--
	}

	end := offset
	for end < len(text) && isIdentChar(text[end]) {
		end++
	}

	return text[start:end]
}

func isIdentChar(c byte) bool {
	return c == '.' || c == '_' ||
		unicode.IsLetter(rune(c)) || unicode.IsDigit(rune(c))
}

// definitionNameOffset finds the offset of the LET variable name in
// the statement "LET <name> = ...".
func definitionNameOffset(text string, def vfilter.DefinitionSite) int {
	pos := def.Pos.Pos.Offset
	if pos < 0 || pos >= len(text) {
		return -1
	}

	body := text[pos:]

	// Skip the "LET" keyword, whitespace and comments.
	i := 0
	for i < len(body) && unicode.IsLetter(rune(body[i])) {
		i++
	}
	for i < len(body) {
		if unicode.IsSpace(rune(body[i])) {
			i++
			continue
		}
		if strings.HasPrefix(body[i:], "/*") {
			end := strings.Index(body[i+2:], "*/")
			if end < 0 {
				return -1
			}
			i += 2 + end + 2
			continue
		}
		break
	}

	if i+len(def.Name) > len(body) {
		return -1
	}
	return pos + i
}
