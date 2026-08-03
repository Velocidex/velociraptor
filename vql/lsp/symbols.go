/*
   Velociraptor - Dig Deeper
   Copyright (C) 2019-2025 Rapid7 Inc.

   This program is free software: you can redistribute it and/or modify
   it under the terms of the GNU Affero General Public License as published
   by the Free Software Foundation, either version 3 of the License, or
   (at your option) any later version.

   This program is distributed in the hope that it will be useful,
   but WITHOUT ANY WARRANTY; without even the implied warranty of
   MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
   GNU Affero General Public License for more details.

   You should have received a copy of the GNU Affero General Public License
   along with this program.  If not, see <https://www.gnu.org/licenses/>.
*/

package lsp

import (
	"context"

	"go.lsp.dev/protocol"
	"www.velocidex.com/golang/vfilter"
)

// DocumentSymbol implements textDocument/documentSymbol - a hierarchical
// outline of the query document: LET definitions, queries with their
// plugins, columns and nested function calls.
//
// We advertise DocumentSymbolProvider in Initialize, and return a
// hierarchical []DocumentSymbol tree (kind + nested children), which
// clients use for breadcrumbs, outline views and go-to-symbol.
func (self *Server) DocumentSymbol(
	ctx context.Context, params *protocol.DocumentSymbolParams) (protocol.DocumentSymbolResult, error) {

	self.mu.Lock()
	text, pres := self.documents[params.TextDocument.URI]
	self.mu.Unlock()

	if !pres {
		return protocol.DocumentSymbolSlice{}, nil
	}

	statements, err := vfilter.MultiParseWithComments(text)
	if err != nil {
		// If the document does not parse we return an empty outline
		// rather than erroring - diagnostics will explain the syntax
		// problem.
		return protocol.DocumentSymbolSlice{}, nil
	}

	line_starts := lineStarts(text)
	mapper := positionMapper{
		document:    text,
		line_starts: line_starts,
	}

	var symbols []protocol.DocumentSymbol
	for _, statement := range statements {
		outline := vfilter.Outline(statement)
		if outline == nil {
			continue
		}
		symbols = append(symbols, self.outlineToSymbol(outline, text, mapper))
	}

	return protocol.DocumentSymbolSlice(symbols), nil
}

// outlineToSymbol converts a vfilter OutlineInfo node into an LSP
// DocumentSymbol, recursing into children.
func (self *Server) outlineToSymbol(
	info *vfilter.OutlineInfo, document string, mapper positionMapper) protocol.DocumentSymbol {

	symbol := protocol.DocumentSymbol{
		Name:          self.outlineName(info, document, mapper),
		Kind:          outlineKindToSymbolKind(info.Kind),
		Range:         mapper.rangeOf(info.Pos, info.EndPos),
		SelectionRange: mapper.rangeOf(info.Pos, info.EndPos),
		Children:      []protocol.DocumentSymbol{},
	}

	for _, child := range info.Children {
		symbol.Children = append(symbol.Children,
			self.outlineToSymbol(child, document, mapper))
	}
	return symbol
}

// outlineName returns the display name for an outline node. Most nodes
// carry a name; unaliased columns do not, so we extract the source text
// between the node's offsets.
func (self *Server) outlineName(
	info *vfilter.OutlineInfo, document string, mapper positionMapper) string {

	if info.Name != "" {
		return info.Name
	}

	// Extract the source text of the node for a human-readable name.
	start := info.Pos.Offset
	end := info.EndPos.Offset
	if start < 0 {
		start = 0
	}
	if end <= start || end > len(document) {
		return "<expression>"
	}
	return document[start:end]
}

// outlineKindToSymbolKind maps vfilter outline kinds to LSP SymbolKinds.
func outlineKindToSymbolKind(kind vfilter.OutlineKind) protocol.SymbolKind {
	switch kind {
	case vfilter.OutlineKindLet:
		return protocol.SymbolKindVariable
	case vfilter.OutlineKindQuery:
		return protocol.SymbolKindFunction
	case vfilter.OutlineKindColumn:
		return protocol.SymbolKindField
	case vfilter.OutlineKindFunction:
		return protocol.SymbolKindFunction
	default:
		return protocol.SymbolKindFile
	}
}
