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

// maxParseErrors caps how many syntax errors we report from a single
// document. Each error triggers a re-parse of a truncated segment so
// this also bounds the number of retries.
const maxParseErrors = 50

// Validate parses a VQL document and returns diagnostics.
//
// It reports:
//   - syntax errors (with precise line/column)
//   - unknown plugins in FROM clauses
//   - unknown function calls
//   - unknown keyword arguments on known plugins/functions
//
// Participle is a whole-document parser: on the first syntax error it
// returns nil and discards the entire AST. To keep diagnostics useful
// for half-typed or partially wrong queries we use truncate-and-retry:
// when a segment fails to parse we report the error, validate the
// largest valid prefix before it, then continue from the next line to
// look for more errors.
func (self *Registry) Validate(document string) []protocol.Diagnostic {
	diagnostics := []protocol.Diagnostic{}
	defined := make(map[string]bool)
	line_starts := lineStarts(document)

	base := 0
	errors_found := 0
	for base < len(document) && errors_found < maxParseErrors {
		segment := document[base:]

		statements, err := vfilter.MultiParseWithComments(segment)
		if err == nil {
			// Clean parse of the rest of the document.
			mapper := positionMapper{
				document:    document,
				line_starts: line_starts,
				base:        base,
			}
			self.validateStatements(statements, defined, mapper, &diagnostics)
			break
		}

		// The segment failed to parse. Find where.
		abs, message := parseErrorPosition(err, base)
		if abs < 0 {
			// No position information at all - report and stop.
			diagnostics = append(diagnostics, fallbackDiagnostic(err))
			break
		}

		diagnostics = append(diagnostics, syntaxDiagnostic(
			document, line_starts, abs, message))
		errors_found++

		rel := abs - base

		// Validate the largest valid prefix before the error. It
		// starts at the same base so positions map to the document
		// exactly. If truncating exactly at the error fails (the
		// error may be mid-statement) we back up to the previous
		// statement boundary.
		if prefix_statements := largestParseablePrefix(segment, rel); prefix_statements != nil {
			mapper := positionMapper{
				document:    document,
				line_starts: line_starts,
				base:        base,
			}
			self.validateStatements(
				prefix_statements, defined, mapper, &diagnostics)
		}

		// Advance past the error, skipping to the next line so we
		// don't re-trip on the same construct.
		next := abs
		for next < len(document) && document[next] != '\n' {
			next++
		}
		if next < len(document) {
			next++ // skip the newline itself
		}
		if next <= abs {
			next = abs + 1
		}
		base = next
	}

	return diagnostics
}

// validateStatements collects LET definitions from all statements in the
// segment, then validates calls and symbols against them.
func (self *Registry) validateStatements(
	statements []*vfilter.VQL, defined map[string]bool,
	mapper positionMapper, diagnostics *[]protocol.Diagnostic) {

	for _, statement := range statements {
		inspection := vfilter.Inspect(statement)
		for _, let := range inspection.Lets {
			defined[let.Name] = true
		}
	}

	for _, statement := range statements {
		inspection := vfilter.Inspect(statement)
		for _, call := range inspection.Calls {
			self.validateCall(call, defined, mapper, diagnostics)
		}
		for _, symbol := range inspection.Symbols {
			self.validateSymbol(symbol, defined, mapper, diagnostics)
		}
	}
}

// validateCall checks a plugin/function call site against the registry.
func (self *Registry) validateCall(
	call vfilter.CallInfo, defined map[string]bool,
	mapper positionMapper, diagnostics *[]protocol.Diagnostic) {

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
			Range:    mapper.rangeOf(call.Pos, call.EndPos),
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
				Range:    mapper.rangeOf(arg.Pos, arg.EndPos),
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
// or function names used without call parens. Columns are dynamic so we
// never flag them; we only resolve against known LET vars and functions.
func (self *Registry) validateSymbol(
	symbol vfilter.SymbolInfo, defined map[string]bool,
	mapper positionMapper, diagnostics *[]protocol.Diagnostic) {

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

// largestParseablePrefix returns the largest prefix of segment[:end] that
// parses cleanly, backing up to previous statement boundaries (newline or
// semicolon) if the exact prefix fails to parse. Returns nil if no
// non-empty prefix parses.
func largestParseablePrefix(segment string, end int) []*vfilter.VQL {
	for end > 0 {
		prefix := segment[:end]
		statements, err := vfilter.MultiParseWithComments(prefix)
		if err == nil {
			return statements
		}

		// Back up to the previous statement boundary.
		newline := strings.LastIndex(prefix, "\n")
		semicolon := strings.LastIndex(prefix, ";")
		boundary := newline
		if semicolon > boundary {
			boundary = semicolon
		}
		if boundary <= 0 {
			// Only whole-line candidates left; stop.
			return nil
		}
		end = boundary
	}
	return nil
}

// positionMapper converts lexer positions that are relative to a segment
// of the document (starting at byte offset base) into absolute LSP
// positions in the document.
type positionMapper struct {
	document    string
	line_starts []int
	base        int
}

func (self positionMapper) mapPos(pos lexer.Position) protocol.Position {
	// Prefer the byte offset - it survives truncation exactly.
	if pos.Offset > 0 {
		return offsetToPosition(self.document, self.line_starts, self.base+pos.Offset)
	}
	// Fall back to line/column math.
	line := uint32(0)
	if pos.Line > 0 {
		line = uint32(pos.Line - 1)
	}
	col := uint32(0)
	if pos.Column > 0 {
		col = uint32(pos.Column - 1)
	}
	return protocol.Position{Line: line, Character: col}
}

func (self positionMapper) rangeOf(pos, end_pos lexer.Position) protocol.Range {
	return protocol.Range{
		Start: self.mapPos(pos),
		End:   self.mapPos(end_pos),
	}
}

// parseErrorPosition unwraps a vfilter parse error and returns the
// absolute byte offset of the failure (document-relative, given base) and
// its message. Returns abs=-1 if no position could be recovered.
func parseErrorPosition(err error, base int) (int, string) {
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
		return -1, ""
	}

	if pos.Offset > 0 {
		return base + pos.Offset, message
	}
	if pos.Line > 0 && pos.Column > 0 {
		// No byte offset; approximate from line/column (rare).
		// Fall back to the offset of the line start.
		return -1, message
	}
	return -1, message
}

// syntaxDiagnostic builds a syntax error diagnostic at an absolute byte
// offset in the document.
func syntaxDiagnostic(document string, line_starts []int, abs int, message string) protocol.Diagnostic {
	start := offsetToPosition(document, line_starts, abs)
	end := start
	end.Character++

	return protocol.Diagnostic{
		Range:    protocol.Range{Start: start, End: end},
		Severity: protocol.DiagnosticSeverityError,
		Source:   protocol.NewOptional("vql"),
		Message:  protocol.String(message),
	}
}

// fallbackDiagnostic builds a diagnostic when no position information is
// available at all.
func fallbackDiagnostic(err error) protocol.Diagnostic {
	return protocol.Diagnostic{
		Range:    protocol.Range{Start: protocol.Position{}, End: protocol.Position{}},
		Severity: protocol.DiagnosticSeverityError,
		Source:   protocol.NewOptional("vql"),
		Message:  protocol.String(err.Error()),
	}
}

// lineStarts returns the byte offset of the start of each line in the
// document. line_starts[i] is the offset of the first byte of line i
// (0-based).
func lineStarts(document string) []int {
	starts := []int{0}
	for i := 0; i < len(document); i++ {
		if document[i] == '\n' {
			starts = append(starts, i+1)
		}
	}
	return starts
}

// offsetToPosition converts an absolute byte offset into a 0-based LSP
// line/column position. Column is measured in bytes from the line start,
// which matches participle for ASCII input.
func offsetToPosition(document string, line_starts []int, abs int) protocol.Position {
	if abs < 0 {
		abs = 0
	}
	if abs > len(document) {
		abs = len(document)
	}

	// Binary search for the line containing abs.
	lo, hi := 0, len(line_starts)-1
	for lo < hi {
		mid := (lo + hi + 1) / 2
		if line_starts[mid] <= abs {
			lo = mid
		} else {
			hi = mid - 1
		}
	}

	return protocol.Position{
		Line:      uint32(lo),
		Character: uint32(abs - line_starts[lo]),
	}
}
