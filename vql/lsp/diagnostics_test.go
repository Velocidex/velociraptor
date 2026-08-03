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
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.lsp.dev/protocol"
)

// newTestRegistry builds a small registry with just enough plugins and
// functions to exercise the validator without a full Velociraptor scope.
func newTestRegistry() *Registry {
	return &Registry{
		plugins: map[string]*Callable{
			"pslist": {
				Name: "pslist",
				Type: "plugin",
				Args: []Arg{{Name: "pid", Type: "int"}},
			},
			"Artifact.Linux.Sys.Users": {
				Name: "Artifact.Linux.Sys.Users",
				Type: "plugin",
			},
		},
		functions: map[string]*Callable{
			"upcase": {
				Name: "upcase",
				Type: "function",
				Args: []Arg{{Name: "str", Type: "string"}},
			},
		},
	}
}

func TestValidateCleanDocument(t *testing.T) {
	registry := newTestRegistry()

	diagnostics := registry.Validate(
		"SELECT * FROM pslist(pid=1) WHERE Name =~ 'foo'")
	assert.Empty(t, diagnostics)
}

func TestValidateUnknownFunction(t *testing.T) {
	registry := newTestRegistry()

	diagnostics := registry.Validate(
		"SELECT upcase(str='x'), unknownfunc() FROM pslist()")
	require.Len(t, diagnostics, 1)
	assert.Equal(t, "Unknown function 'unknownfunc'", messageOf(diagnostics[0]))
	assert.Equal(t, uint32(0), diagnostics[0].Range.Start.Line)
	// "SELECT upcase(str='x'), " is 24 characters.
	assert.Equal(t, uint32(24), diagnostics[0].Range.Start.Character)
}

func TestValidateUnknownPlugin(t *testing.T) {
	registry := newTestRegistry()

	diagnostics := registry.Validate("SELECT * FROM bogusplugin()")
	require.Len(t, diagnostics, 1)
	assert.Equal(t, "Unknown plugin 'bogusplugin'", messageOf(diagnostics[0]))
}

func TestValidateUnknownArgument(t *testing.T) {
	registry := newTestRegistry()

	diagnostics := registry.Validate("SELECT * FROM pslist(foo=1)")
	require.Len(t, diagnostics, 1)
	assert.Equal(t,
		"Unknown argument 'foo' for plugin 'pslist'", messageOf(diagnostics[0]))
}

func TestValidateMultilinePositions(t *testing.T) {
	registry := newTestRegistry()

	// Line 1 (0-based) contains the offending call.
	document := "SELECT * FROM pslist(pid=1)\n" +
		"SELECT upcase(str='x'), bogusfunc() FROM pslist()\n"
	diagnostics := registry.Validate(document)
	require.Len(t, diagnostics, 1)
	assert.Equal(t, uint32(1), diagnostics[0].Range.Start.Line)
	// "SELECT upcase(str='x'), " is 24 characters.
	assert.Equal(t, uint32(24), diagnostics[0].Range.Start.Character)
}

func TestValidateSyntaxErrorOnly(t *testing.T) {
	registry := newTestRegistry()

	// The whole document fails to parse; we get the syntax error and
	// nothing else since nothing before it is valid.
	diagnostics := registry.Validate("SELECT * FROM")
	require.Len(t, diagnostics, 1)
	assert.Contains(t, messageOf(diagnostics[0]), "unexpected token")
}

// TestValidateTruncateAndRetry checks the whole-document-parse-failure
// mitigation: when a document has a syntax error in the middle we still
// validate valid statements before and after it.
func TestValidateTruncateAndRetry(t *testing.T) {
	registry := newTestRegistry()

	document := "SELECT * FROM pslist(foo=1)\n" + // bad arg (before error)
		"SELECT @@@\n" + // syntax error
		"SELECT * FROM pslist(bar=2)\n" + // bad arg (after error)
		"SELECT * FROM bogusplugin()\n" // unknown plugin (after error)
	diagnostics := registry.Validate(document)

	messages := make(map[string]bool)
	for _, d := range diagnostics {
		messages[messageOf(d)] = true
	}

	// We should see the syntax error plus all four semantic errors.
	assert.True(t, messages["Unknown argument 'foo' for plugin 'pslist'"],
		"missing bad arg before the syntax error")
	assert.True(t, messages["Unknown argument 'bar' for plugin 'pslist'"],
		"missing bad arg after the syntax error")
	assert.True(t, messages["Unknown plugin 'bogusplugin'"],
		"missing unknown plugin after the syntax error")

	found_syntax := false
	for _, d := range diagnostics {
		if contains(d, "unexpected token") || contains(d, "invalid input") ||
			contains(d, "lexer:") {
			found_syntax = true
		}
	}
	assert.True(t, found_syntax, "missing syntax error diagnostic")
}

// TestValidateTruncateAndRetrySyntaxErrorPosition checks that the syntax
// error diagnostic is positioned at the right line even on later lines.
func TestValidateTruncateAndRetrySyntaxErrorPosition(t *testing.T) {
	registry := newTestRegistry()

	document := "SELECT * FROM pslist(pid=1)\n" +
		"SELECT * FROM pslist(pid=2)\n" +
		"SELECT @@@\n"
	diagnostics := registry.Validate(document)

	require.Len(t, diagnostics, 1)
	assert.Equal(t, uint32(2), diagnostics[0].Range.Start.Line)
	assert.Equal(t, uint32(7), diagnostics[0].Range.Start.Character)
}

// TestValidateArtifactArgument checks that an unknown keyword argument on an
// artifact (registered via AddArtifact) is reported.
func TestValidateArtifactArgument(t *testing.T) {
	registry := newTestRegistry()
	registry.AddArtifact("Artifact.Windows.Sys.Users",
		"Enumerate users",
		[]Arg{{Name: "remoteRegKey", Type: "string"}})

	// Valid parameter - clean.
	diagnostics := registry.Validate(
		"SELECT * FROM Artifact.Windows.Sys.Users()")
	assert.Empty(t, diagnostics)

	// Unknown parameter - reported.
	diagnostics = registry.Validate(
		"SELECT * FROM Artifact.Windows.Sys.Users(foo=1)")
	require.Len(t, diagnostics, 1)
	assert.Equal(t, "Unknown argument 'foo' for artifact "+
		"'Artifact.Windows.Sys.Users'", messageOf(diagnostics[0]))
	assert.Equal(t, uint32(41), diagnostics[0].Range.Start.Character)
}

// TestValidateKnownArtifactParameters checks that all real parameters of an
// artifact pass validation.
func TestValidateKnownArtifactParameters(t *testing.T) {
	registry := newTestRegistry()
	registry.AddArtifact("Artifact.Generic.Client.VQL",
		"Run a VQL query",
		[]Arg{{Name: "Command", Type: "string"}})

	diagnostics := registry.Validate(
		"SELECT * FROM Artifact.Generic.Client.VQL(Command='SELECT 5')")
	assert.Empty(t, diagnostics)
}

func messageOf(d protocol.Diagnostic) string {
	switch m := d.Message.(type) {
	case protocol.String:
		return string(m)
	case *protocol.MarkupContent:
		return m.Value
	}
	return ""
}

func contains(d protocol.Diagnostic, substr string) bool {
	return len(messageOf(d)) > 0 &&
		len(substr) > 0 &&
		messageOf(d)[:min(len(messageOf(d)), len(substr))] == substr
}
