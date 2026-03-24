package utils

import (
	"runtime"
	"testing"

	"www.velocidex.com/golang/velociraptor/vtesting/assert"
)

var (
	joinTests = []struct {
		base, name string
		out        string
	}{
		{"", "", ""},

		// Directory traversal not allowed.
		{"/bin/ls", "../../foo", "/bin/ls/foo"},

		// Collapse multiple //
		{"/bin/ls", "//////C:foo///bar", "/bin/ls/C%3Afoo/bar"},

		{"C:\\Windows", "/System32/notepad.exe", "C:\\Windows/System32/notepad.exe"},
	}
)

func TestJoin(t *testing.T) {
	if runtime.GOOS == "linux" {
		for _, tc := range joinTests {
			out := Join(tc.base, tc.name)
			assert.Equal(t, out, tc.out)
		}
	}
}
