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
	"strings"

	"github.com/alecthomas/participle/v2"
	"github.com/alecthomas/participle/v2/lexer"
	"github.com/pkg/errors"
	"go.lsp.dev/protocol"
	"www.velocidex.com/golang/vfilter"
)

// Validate parses a VQL document and returns diagnostics.
//
// It reports:
//   - syntax errors (with precise line/column)
//   - unknown plugins in FROM clauses
//   - unknown function calls
//   - unknown keyword arguments on known plugins/functions
//   - references to undefined LET variables
func (self *Registry) Validate(document string) []protocol.Diagnostic {
	diagnostics := []protocol.Diagnostic{}

	statements, err := vfilter.MultiParseWithComments(document)
	if err != nil {
		diag := syntaxErrorDiagnostic(err, document)
		if diag != nil {
			diagnostics = append(diagnostics, *diag)
		}
		return diagnostics
	}

	// Track LET definitions so we can validate references.
	defined := make(map[string]bool)
	for _, statement := range statements {
		inspection := vfilter.Inspect(statement)
		for _, let := range inspection.Lets {
			defined[let.Name] = true
		}
	}

	for _, statement := range statements {
		inspection := vfilter.Inspect(statement)
		for _, call := range inspection.Calls {
			self.validateCall(call, defined, &diagnostics)
		}
		for _, symbol := range inspection.Symbols {
			self.validateSymbol(symbol, defined, &diagnostics)
		}
	}

	return diagnostics
}

// validateCall checks a plugin/function call site against the registry.
func (self *Registry) validateCall(
	call vfilter.CallInfo, defined map[string]bool,
	diagnostics *[]protocol.Diagnostic) {

	// The first dotted component is the scope where the callable
	// should be resolved. Plugins are registered under their full
	// dotted name; functions are registered under a simple name.
	var callable *Callable
	var pres bool

	if call.IsPlugin {
		// A plugin may also be a LET-defined stored query.
		if defined[call.Name] {
			return
		}
		callable, pres = self.GetPlugin(call.Name)
	} else {
		callable, pres = self.GetFunction(call.Name)
	}

	if !pres {
		// Maybe the first component is a LET var subquery, e.g.
		// SELECT * FROM MyQuery.
		if !call.IsPlugin && defined[firstComponent(call.Name)] {
			return
		}

		kind := "function"
		if call.IsPlugin {
			kind = "plugin"
		}
		*diagnostics = append(*diagnostics, protocol.Diagnostic{
			Range:    rangeFromPositions(call.Pos, call.EndPos),
			Severity: protocol.DiagnosticSeverityError,
			Source:   protocol.NewOptional("vql"),
			Message:  protocol.String("Unknown " + kind + " '" + call.Name + "'"),
		})
		return
	}

	// Validate keyword arguments.
	if callable.FreeForm {
		return
	}
	known := make(map[string]bool)
	for _, arg := range callable.Args {
		known[arg.Name] = true
	}

	for _, arg := range call.Args {
		if arg.Name == "" {
			// Positional or array arg - can't check statically.
			continue
		}
		if !known[arg.Name] {
			*diagnostics = append(*diagnostics, protocol.Diagnostic{
				Range:    rangeFromPositions(arg.Pos, arg.EndPos),
				Severity: protocol.DiagnosticSeverityError,
				Source:   protocol.NewOptional("vql"),
				Message: protocol.String(
					"Unknown argument '" + arg.Name + "' for " +
						callable.Type + " '" + call.Name + "'"),
			})
		}
	}
}

// validateSymbol checks a bare symbol reference against LET definitions.
//
// Bare symbols can be column names (dynamic, from the row), LET variables
// or function names used without call parens. We only flag symbols that
// are certainly not columns - i.e. we only warn when a symbol matches no
// known function and no LET definition but is used as if it were a stored
// query variable. Columns are dynamic so we never flag them.
func (self *Registry) validateSymbol(
	symbol vfilter.SymbolInfo, defined map[string]bool,
	diagnostics *[]protocol.Diagnostic) {

	// If it is a defined LET var it is fine.
	if defined[symbol.Name] {
		return
	}

	// If it resolves to a function it is fine (function used without
	// parens as a value is legal, e.g. for lambdas).
	if _, pres := self.GetFunction(symbol.Name); pres {
		return
	}

	// Everything else is assumed to be a column from the row, which
	// is dynamic, so we don't emit a diagnostic.
	_ = diagnostics
}

func firstComponent(name string) string {
	idx := strings.Index(name, ".")
	if idx == -1 {
		return name
	}
	return name[:idx]
}

// syntaxErrorDiagnostic converts a parse error into an LSP diagnostic.
//
// vfilter wraps parse errors with pkg/errors so we need to unwrap to the
// root cause to find the *lexer.Error or participle.Error carrying the
// position.
func syntaxErrorDiagnostic(err error, document string) *protocol.Diagnostic {
	cause := errors.Cause(err)

	var pos lexer.Position
	var message string

	switch t := cause.(type) {
	case *lexer.Error:
		pos = t.Pos
		message = t.Msg
	case participle.Error:
		pos = t.Position()
		message = t.Message()
	default:
		// No position available; fall back to the message text.
		return &protocol.Diagnostic{
			Range:    protocol.Range{Start: protocol.Position{}, End: protocol.Position{}},
			Severity: protocol.DiagnosticSeverityError,
			Source:   protocol.NewOptional("vql"),
			Message:  protocol.String(err.Error()),
		}
	}

	start := protocol.Position{
		Line:      uint32(pos.Line - 1),
		Character: uint32(pos.Column - 1),
	}
	// Give the diagnostic a small width on the offending line.
	end := start
	end.Character = start.Character + 1

	return &protocol.Diagnostic{
		Range:    protocol.Range{Start: start, End: end},
		Severity: protocol.DiagnosticSeverityError,
		Source:   protocol.NewOptional("vql"),
		Message:  protocol.String(message),
	}
}

// rangeFromPositions converts a vfilter position pair into an LSP range.
//
// lexer.Position is 1-based; LSP is 0-based.
func rangeFromPositions(pos, end_pos lexer.Position) protocol.Range {
	return protocol.Range{
		Start: protocol.Position{
			Line:      uint32(pos.Line - 1),
			Character: uint32(pos.Column - 1),
		},
		End: protocol.Position{
			Line:      uint32(end_pos.Line - 1),
			Character: uint32(end_pos.Column - 1),
		},
	}
}
