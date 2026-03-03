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
		{"macOS Finder path with spaces",
			"'/path/to/my file.json'", "/path/to/my file.json"},
		{"macOS Finder path without spaces",
			"'/usr/bin/ls'", "/usr/bin/ls"},
		{"Unquoted path unchanged",
			"/usr/bin/ls", "/usr/bin/ls"},
		{"Empty quoted string",
			"''", ""},
		{"Single quote char unchanged",
			"'", "'"},
		{"Apostrophe in middle unchanged",
			"it's a file", "it's a file"},
		{"Double quotes unchanged",
			`"/path/to/file"`, `"/path/to/file"`},
		{"Empty string unchanged",
			"", ""},
		{"Double quotes inside single quotes",
			`'/path/with "double" quotes/file'`,
			`/path/with "double" quotes/file`},
		{"Mismatched quotes unchanged",
			"'/path/to/file\"", "'/path/to/file\""},

		// Adversarial / edge cases
		{"Path traversal in quotes",
			"'../../etc/passwd'", "../../etc/passwd"},
		{"Triple quotes",
			"'''", "'"},
		{"Double-then-single quote prefix",
			"''/etc/passwd'", "'/etc/passwd"},
		{"Null byte inside quotes",
			"'\x00/path'", "\x00/path"},
		{"Newlines inside quotes",
			"'/path/to/file\nwith\nnewlines'", "/path/to/file\nwith\nnewlines"},
		{"Minimal quoted content",
			"'a'", "a"},
		{"Long path in quotes",
			"'" + longPath + "'", longPath},
		{"Single space inside quotes",
			"' '", " "},
		{"Leading and trailing spaces preserved",
			"'  /path/with/leading/spaces  '", "  /path/with/leading/spaces  "},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := MaybeStripWrappingQuotes(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}
