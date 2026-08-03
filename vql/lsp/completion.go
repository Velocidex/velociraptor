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
	"sort"
	"strings"

	"go.lsp.dev/protocol"
	"www.velocidex.com/golang/vfilter"
)

// Completion returns a list of completion items for the given position.
//
// The server suggests:
//   - registered plugins and functions (matching the typed prefix),
//   - artifact names registered as plugins,
//   - LET variables defined earlier in the document,
//   - argument names when the cursor is inside a call's argument list.
func (self *Server) Completion(
	ctx context.Context, params *protocol.CompletionParams) (protocol.CompletionResult, error) {

	self.mu.Lock()
	document, pres := self.documents[params.TextDocument.URI]
	self.mu.Unlock()
	if !pres {
		return protocol.CompletionItemSlice{}, nil
	}

	offset := positionToOffset(lineStarts(document), params.Position)
	if offset < 0 || offset > len(document) {
		return protocol.CompletionItemSlice{}, nil
	}

	prefix := wordPrefix(document, offset)
	items := []protocol.CompletionItem{}

	// LET variables defined in this document.
	for _, name := range self.letNames(document) {
		if strings.HasPrefix(name, prefix) {
			items = append(items, protocol.CompletionItem{
				Label:  name,
				Kind:   protocol.CompletionItemKindVariable,
				Detail: protocol.NewOptional("LET variable"),
			})
		}
	}

	// Plugins and functions from the registry. The registry is
	// immutable after startup so concurrent reads are safe.
	for name, callable := range self.registry.AllCallables() {
		if strings.HasPrefix(name, prefix) {
			detail := callable.Type
			if callable.IsAggregate {
				detail += " (aggregate)"
			}
			items = append(items, protocol.CompletionItem{
				Label:  name,
				Kind:   protocol.CompletionItemKindFunction,
				Detail: protocol.NewOptional(detail),
			})
		}
	}

	// Argument names for the enclosing call, if the cursor is inside one.
	for _, name := range self.argumentNames(document, offset) {
		if strings.HasPrefix(name, prefix) {
			items = append(items, protocol.CompletionItem{
				Label:  name,
				Kind:   protocol.CompletionItemKindField,
				Detail: protocol.NewOptional("argument"),
			})
		}
	}

	sort.Slice(items, func(i, j int) bool {
		return items[i].Label < items[j].Label
	})

	return protocol.CompletionItemSlice(items), nil
}

// letNames returns the names of LET variables defined in the document.
func (self *Server) letNames(document string) []string {
	names := []string{}
	statements, err := vfilter.MultiParseWithComments(document)
	if err != nil {
		// The document may end in a partially typed statement
		// (completion is usually requested mid-typing). Recover the
		// largest parseable prefix and collect LETs from that.
		statements = largestParseablePrefix(document, len(document))
	}
	seen := make(map[string]bool)
	for _, statement := range statements {
		inspection := vfilter.Inspect(statement)
		for _, let := range inspection.Lets {
			if !seen[let.Name] {
				seen[let.Name] = true
				names = append(names, let.Name)
			}
		}
	}
	return names
}

// argumentNames returns the argument names of the call enclosing the given
// byte offset, if any. It scans backwards for the opening paren of the
// innermost call containing the cursor, then resolves the callable name
// before that paren against the registry.
func (self *Server) argumentNames(document string, offset int) []string {
	// Find the last '(' before the cursor.
	open := strings.LastIndex(document[:offset], "(")
	if open < 0 {
		return nil
	}
	// Make sure we are not past the closing paren of a call (i.e. we are
	// inside the argument list, not after it).
	close_idx := strings.Index(document[open:offset], ")")
	if close_idx >= 0 && close_idx < offset-open {
		return nil
	}

	// The callable name is the identifier chain just before '('.
	start := open
	for start > 0 {
		c := document[start-1]
		if c == '.' || c == '_' || (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') ||
			(c >= '0' && c <= '9') {
			start--
			continue
		}
		break
	}
	name := strings.TrimSpace(document[start:open])

	var callable *Callable
	var pres bool

	// Only attempt argument completion for known callables.
	callable, pres = self.registry.GetFunction(name)
	if !pres {
		callable, pres = self.registry.GetPlugin(name)
	}
	if !pres || callable.FreeForm {
		return nil
	}

	result := make([]string, 0, len(callable.Args))
	for _, arg := range callable.Args {
		result = append(result, arg.Name)
	}
	return result
}

// wordPrefix returns the identifier prefix ending at the given offset.
// Identifiers in VQL can contain letters, digits, underscores and dots
// (for dotted plugin names like Artifact.Linux.Sys.Users).
func wordPrefix(document string, offset int) string {
	start := offset
	for start > 0 {
		c := document[start-1]
		if c == '.' || c == '_' || (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') ||
			(c >= '0' && c <= '9') {
			start--
			continue
		}
		break
	}
	return document[start:offset]
}
