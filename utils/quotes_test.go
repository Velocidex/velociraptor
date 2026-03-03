package utils

import (
	"strings"
	"testing"

	"www.velocidex.com/golang/velociraptor/vtesting/assert"
)

var longPath = "/" + strings.Repeat("a", 999)

func TestMaybeStripWrappingQuotes(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		// macOS Finder "Copy as Pathname" cases
		{"macOS Finder path with spaces",
			"'/path/to/my file.json'", "/path/to/my file.json"},
		{"macOS Finder path without spaces",
			"'/usr/bin/ls'", "/usr/bin/ls"},
		{"Double quotes inside single quotes",
			`'/path/with "double" quotes/file'`,
			`/path/with "double" quotes/file`},
		{"Long path in quotes",
			"'" + longPath + "'", longPath},

		// Paths that must NOT be stripped
		{"Unquoted path unchanged",
			"/usr/bin/ls", "/usr/bin/ls"},
		{"Apostrophe in middle unchanged",
			"it's a file", "it's a file"},
		{"Double quotes unchanged",
			`"/path/to/file"`, `"/path/to/file"`},
		{"Empty string unchanged",
			"", ""},
		{"Single quote char unchanged",
			"'", "'"},
		{"Mismatched quotes unchanged",
			"'/path/to/file\"", "'/path/to/file\""},
		{"Empty quoted string unchanged",
			"''", "''"},
		{"Relative path with quotes preserved",
			"'relative/path'", "'relative/path'"},
		{"Quoted filename preserved",
			"'myfile'", "'myfile'"},
		{"Single space inside quotes unchanged",
			"' '", "' '"},

		// Adversarial / edge cases
		{"Path traversal in quotes stripped",
			"'/../etc/passwd'", "/../etc/passwd"},
		{"Triple quotes unchanged",
			"'''", "'''"},
		{"Null byte after slash stripped",
			"'/\x00path'", "/\x00path"},
		{"Newlines inside quotes stripped",
			"'/path/to/file\nwith\nnewlines'", "/path/to/file\nwith\nnewlines"},
		{"Leading spaces after slash preserved",
			"'/  path/with/leading/spaces  '", "/  path/with/leading/spaces  "},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := MaybeStripWrappingQuotes(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}
