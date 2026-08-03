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
	"fmt"
	"strings"

	"github.com/alecthomas/participle/v2/lexer"
	"go.lsp.dev/protocol"
	"www.velocidex.com/golang/vfilter"
)

// Hover returns documentation and signature information for the symbol
// under the cursor.
//
// Supported targets:
//   - a plugin or function call (documentation + argument list)
//   - an argument name inside a call (the argument's type)
//   - a LET variable definition
//   - a symbol reference that resolves to a LET variable or known function
func (self *Server) Hover(ctx context.Context, params *protocol.HoverParams) (*protocol.Hover, error) {
	self.mu.Lock()
	document, pres := self.documents[params.TextDocument.URI]
	self.mu.Unlock()
	if !pres {
		return nil, nil
	}

	line_starts := lineStarts(document)
	offset := positionToOffset(line_starts, params.Position)

	statements, err := vfilter.MultiParseWithComments(document)
	if err != nil {
		return nil, nil
	}

	mapper := positionMapper{document: document, line_starts: line_starts}

	for _, statement := range statements {
		inspection := vfilter.Inspect(statement)

		// LET variables are the most specific: the name is exact.
		for _, let := range inspection.Lets {
			if containsOffset(let.Pos, let.Pos, offset) {
				return self.letHover(let, mapper), nil
			}
		}

		// Calls - check argument names first (they are more precise),
		// then the call name itself.
		for _, call := range inspection.Calls {
			for _, arg := range call.Args {
				if containsOffset(arg.Pos, arg.EndPos, offset) {
					return self.argHover(call, arg, mapper), nil
				}
			}
			if containsOffset(call.Pos, call.Pos, offset) {
				return self.callHover(call, mapper), nil
			}
		}

		// Bare symbol references.
		for _, symbol := range inspection.Symbols {
			if containsOffset(symbol.Pos, symbol.EndPos, offset) {
				return self.symbolHover(symbol, mapper), nil
			}
		}
	}

	return nil, nil
}

func (self *Server) letHover(let vfilter.LetInfo, mapper positionMapper) *protocol.Hover {
	content := fmt.Sprintf("**LET %s**\n\nA local variable defined in this document.", let.Name)
	return hoverWithRange(mapper.rangeOf(let.Pos, let.Pos), content)
}

func (self *Server) callHover(call vfilter.CallInfo, mapper positionMapper) *protocol.Hover {
	var callable *Callable
	var pres bool
	if call.IsPlugin {
		callable, pres = self.registry.GetPlugin(call.Name)
	} else {
		callable, pres = self.registry.GetFunction(call.Name)
	}

	kind := "function"
	if call.IsPlugin {
		kind = "plugin"
	}

	var content string
	if !pres {
		content = fmt.Sprintf("**%s** (%s)\n\nUnknown %s.", call.Name, kind, kind)
	} else {
		content = callableToMarkdown(callable, kind)
	}

	return hoverWithRange(mapper.rangeOf(call.Pos, call.EndPos), content)
}

func (self *Server) argHover(call vfilter.CallInfo, arg vfilter.ArgInfo,
	mapper positionMapper) *protocol.Hover {

	if arg.Name == "" {
		return nil
	}

	// Resolve the callable to find the argument's type.
	var callable *Callable
	var pres bool
	if call.IsPlugin {
		callable, pres = self.registry.GetPlugin(call.Name)
	} else {
		callable, pres = self.registry.GetFunction(call.Name)
	}

	var content string
	if pres && !callable.FreeForm {
		arg_type := ""
		for _, a := range callable.Args {
			if a.Name == arg.Name {
				arg_type = a.Type
				break
			}
		}
		if arg_type == "" {
			content = fmt.Sprintf("**%s**\n\nUnknown argument.", arg.Name)
		} else {
			content = fmt.Sprintf("**%s** — `%s`", arg.Name, arg_type)
		}
	} else {
		content = fmt.Sprintf("**%s**", arg.Name)
	}

	return hoverWithRange(mapper.rangeOf(arg.Pos, arg.EndPos), content)
}

func (self *Server) symbolHover(symbol vfilter.SymbolInfo,
	mapper positionMapper) *protocol.Hover {

	// A known function used as a value.
	if callable, pres := self.registry.GetFunction(symbol.Name); pres {
		return hoverWithRange(mapper.rangeOf(symbol.Pos, symbol.EndPos),
			callableToMarkdown(callable, "function"))
	}

	// A plugin name referenced without parens.
	if callable, pres := self.registry.GetPlugin(symbol.Name); pres {
		return hoverWithRange(mapper.rangeOf(symbol.Pos, symbol.EndPos),
			callableToMarkdown(callable, "plugin"))
	}

	// Everything else is a dynamic column or an unknown symbol - no
	// information to show.
	return nil
}

// callableToMarkdown renders registry information for a callable as
// markdown documentation.
func callableToMarkdown(callable *Callable, kind string) string {
	var b strings.Builder

	fmt.Fprintf(&b, "**%s** (%s)", callable.Name, kind)
	if callable.IsAggregate {
		b.WriteString(" — *aggregate*")
	}
	b.WriteString("\n")

	if callable.Doc != "" {
		fmt.Fprintf(&b, "\n%s\n", callable.Doc)
	}

	if !callable.FreeForm && len(callable.Args) > 0 {
		b.WriteString("\n**Arguments:**\n")
		for _, arg := range callable.Args {
			if arg.Type == "" {
				fmt.Fprintf(&b, "- `%s`\n", arg.Name)
			} else {
				fmt.Fprintf(&b, "- `%s` — *%s*\n", arg.Name, arg.Type)
			}
		}
	} else if callable.FreeForm {
		b.WriteString("\nAccepts arbitrary keyword arguments.")
	}

	return b.String()
}

func hoverWithRange(rng protocol.Range, content string) *protocol.Hover {
	return &protocol.Hover{
		Contents: &protocol.MarkupContent{
			Kind:  protocol.MarkupKindMarkdown,
			Value: content,
		},
		Range: &rng,
	}
}

// positionToOffset converts an LSP position (0-based line/character) to a
// byte offset in the document.
func positionToOffset(line_starts []int, pos protocol.Position) int {
	if int(pos.Line) >= len(line_starts) {
		return -1
	}
	return line_starts[pos.Line] + int(pos.Character)
}

// containsOffset reports whether offset falls within the half-open byte
// range [start.Offset, end.Offset). If EndPos has no offset, falls back to
// a single-character match at Pos.
func containsOffset(pos, end_pos lexer.Position, offset int) bool {
	if pos.Offset <= 0 {
		return false
	}
	if end_pos.Offset <= pos.Offset {
		return offset >= pos.Offset && offset < pos.Offset+1
	}
	return offset >= pos.Offset && offset < end_pos.Offset
}
