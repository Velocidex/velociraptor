package lsp

import (
	"context"
	"strings"

	"github.com/alecthomas/participle/v2/lexer"
	"go.lsp.dev/protocol"
	"www.velocidex.com/golang/velociraptor/utils"
	"www.velocidex.com/golang/vfilter"
)

// Semantic token types advertised in the legend. The order defines
// the index numbers stored in the token data.
const (
	semTokenKeyword = iota
	semTokenComment
	semTokenString
	semTokenNumber
	semTokenOperator
	semTokenFunction
	semTokenPlugin
	semTokenVariable
	semTokenProperty
)

var tokenTypesLegend = []string{
	"keyword", "comment", "string", "number", "operator",
	"function", "plugin", "variable", "property",
}

var tokenModifiersLegend = []string{}

// lexerTokenToSemanticToken converts a VQL lexer token type to its
// semantic token legend index. Identifiers and SymbolIdents are
// handled separately because they need registry and LET lookup.
func lexerTokenToSemanticToken(token_type string) (int, bool) {
	switch token_type {
	case "Comment", "MLineComment", "VQLComment":
		return semTokenComment, true

	case "String", "MultilineString":
		return semTokenString, true

	case "Number":
		return semTokenNumber, true

	case "Operators":
		return semTokenOperator, true

	case "EXPLAIN", "SELECT", "WHERE", "AND", "OR",
		"AlternativeOR", "AlternativeAND", "FROM", "NOT",
		"AS", "IN", "LIMIT", "NULL", "DESC", "GROUPBY",
		"ORDERBY", "BOOL", "LET":
		return semTokenKeyword, true
	}
	return 0, false
}

// SemanticTokensFull implements textDocument/semanticTokens/full.
func (self *LSPServer) SemanticTokensFull(
	ctx context.Context,
	params *protocol.SemanticTokensParams) (*protocol.SemanticTokens, error) {

	self.mu.Lock()
	doc, pres := self.documents[params.TextDocument.URI]
	self.mu.Unlock()
	if !pres {
		return nil, utils.NotFoundError
	}

	tokens, err := vfilter.Tokenize(doc.Text)
	if err != nil {
		// The document may be temporarily invalid while typing
		// (e.g. a trigger character like '?' or '.'). Return empty
		// tokens instead of failing so the editor keeps working
		// and can clear the old highlighting.
		return &protocol.SemanticTokens{}, nil
	}

	// Build a map of the callable plugins and functions. The API
	// descriptions know the names of built in plugins and functions
	// but not artifact plugins, so we add every call site resolved
	// by the parser (which includes artifacts).
	callables := make(map[string]int)
	for _, desc := range LoadApiDescriptions() {
		if desc == nil {
			continue
		}
		if strings.EqualFold(desc.Type, "Function") {
			callables[desc.Name] = semTokenFunction
		} else {
			callables[desc.Name] = semTokenPlugin
		}
	}

	let_vars, callsites, err := analyseDocument(doc.Text)
	if err != nil {
		let_vars = make(map[string]vfilter.DefinitionSite)
		callsites = nil
	}
	for _, cs := range callsites {
		switch {
		case strings.EqualFold(cs.Type, "function"):
			callables[cs.Name] = semTokenFunction
		case strings.EqualFold(cs.Type, "plugin"):
			callables[cs.Name] = semTokenPlugin
		}
	}

	var data []uint32
	prev_line := uint32(0)
	prev_char := uint32(0)

	appendToken := func(pos lexer.Position, length int, token_type int) {
		if pos.Offset < 0 || length <= 0 {
			return
		}
		pos_proto := protocolPosition(pos)
		if pos_proto.Line != prev_line {
			// The first element of each token tuple is the line
			// delta relative to the previous token, not the
			// absolute line number.
			data = append(data,
				pos_proto.Line-prev_line, pos_proto.Character,
				uint32(length), uint32(token_type), 0)
		} else {
			data = append(data,
				0, pos_proto.Character-prev_char, uint32(length),
				uint32(token_type), 0)
		}
		prev_line = pos_proto.Line
		prev_char = pos_proto.Character
	}

	for i := 0; i < len(tokens); i++ {
		token := tokens[i]

		// Identifiers and bare symbols need semantic lookup to
		// distinguish callables, LET variables and properties.
		if token.Type == "Ident" || token.Type == "SymbolIdent" {
			dotted, end, pres := longestDottedName(tokens, i)
			if pres {
				if token_type, ok := callables[dotted]; ok {
					name_len := dottedTokenLength(tokens, i, end)
					appendToken(token.Pos, name_len,
						token_type)
					i = end
					continue
				}

				if _, pres := let_vars[dotted]; pres {
					name_len := dottedTokenLength(tokens, i, end)
					appendToken(token.Pos, name_len,
						semTokenVariable)
					i = end
					continue
				}

				// Always consume the first name of a known
				// definition lookup.
				if token.Type == "Ident" {
					if _, pres := let_vars[token.Value]; pres {
						appendToken(token.Pos, len(token.Value),
							semTokenVariable)
						continue
					}
				}

				// Everything else is an ordinary property or a
				// bare symbol reference.
				appendToken(token.Pos, len(token.Value),
					semTokenProperty)
				continue
			}

			// A SymbolIdent without a path is a property lookup.
			appendToken(token.Pos, len(token.Value),
				semTokenProperty)
			continue
		}

		token_type, ok := lexerTokenToSemanticToken(token.Type)
		if !ok {
			// Whitespace and any unknown token types are not
			// emitted.
			continue
		}

		appendToken(token.Pos, len(token.Value), token_type)
	}

	return &protocol.SemanticTokens{
		Data: data,
	}, nil
}

// longestDottedName builds a dotted identifier such as
// Artifact.Linux.Sys.Users starting at tokens[i]. It returns the
// name, the index just past the last Ident token consumed, and
// whether a path was found. The scan only continues over explicit
// '.' operator tokens between Ident tokens, so a bare name is
// returned with end == i+1.
func longestDottedName(
	tokens []vfilter.Token, i int) (
	string, int, bool) {

	if i >= len(tokens) {
		return "", i, false
	}

	parts := []string{tokens[i].Value}
	end := i + 1

	for end < len(tokens) {
		// The next token must be a '.' operator.
		if tokens[end].Type != "Operators" || tokens[end].Value != "." {
			break
		}
		// ... followed by an identifier.
		if end+1 >= len(tokens) ||
			tokens[end+1].Type != "Ident" {
			break
		}
		parts = append(parts, tokens[end+1].Value)
		end += 2
	}

	return strings.Join(parts, "."), end, end > i
}

// dottedTokenLength returns the span of all the tokens from i to end,
// including the '.' separators, so semantic tokens cover the full
// dotted name.
func dottedTokenLength(tokens []vfilter.Token, i, end int) int {
	if end > len(tokens) {
		end = len(tokens)
	}
	if i >= end {
		return 0
	}
	last := tokens[end-1]
	return last.Pos.Offset + len(last.Value) - tokens[i].Pos.Offset
}
