package lsp

import (
	"context"
	"strings"

	"github.com/alecthomas/participle/v2/lexer"
	"go.lsp.dev/protocol"
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

type tokenEncoder struct {
	prev protocol.Position
	data []uint32

	document *Document
}

// Encode the new position as a relative position to the old position.
func (self *tokenEncoder) Add(
	lexer_pos lexer.Position, name string, symbol_type string) {

	lexer_type := -1
	// Can be either a function or plugin or something else.
	if symbol_type == "Ident" {
		desc := self.document.getVQLFunctionDescription(name, "function")
		if desc == nil {
			desc = self.document.getVQLFunctionDescription(name, "plugin")
		}
		if desc != nil {
			symbol_type = strings.ToLower(desc.Type)
		}
	}

	lexer_type, ok := lexerTokenToSemanticToken(symbol_type)
	if !ok {
		return
	}
	pos := protocolPosition(lexer_pos)
	column := pos.Character
	if self.prev.Line == pos.Line {
		column = pos.Character - self.prev.Character
	}

	self.data = append(self.data,
		pos.Line-self.prev.Line, column, uint32(len(name)),
		uint32(lexer_type), 0)

	self.prev = pos
}

// SemanticTokensFull implements textDocument/semanticTokens/full.
func (self *LSPServer) SemanticTokensFull(
	ctx context.Context,
	params *protocol.SemanticTokensParams) (*protocol.SemanticTokens, error) {

	doc, err := self.getDoc(params.TextDocument.URI)
	if err != nil {
		return nil, err
	}

	tokens := doc.Tokenize()
	encoder := tokenEncoder{
		document: doc,
	}
	for _, t := range tokens {
		encoder.Add(t.Pos, t.Value, t.Type)
	}

	return &protocol.SemanticTokens{
		Data: encoder.data,
	}, nil
}
