package utils

import (
	"os"
	"testing"

	"www.velocidex.com/golang/velociraptor/vtesting/assert"
)

var (
	fmTests = []struct {
		in     string
		out    os.FileMode
		is_err bool
	}{
		{"", 0, true},

		// Parse as an octal
		{"0755", 0o755, false},

		// Parse as a short string - if the string is short this
		// refers to the owner, then group.

		{"x", 0o700, false}, // Special cased for backwards compatibility

		// Means rwx for owner
		{"rwx", 0o700, false},

		// Means rwx for owner, rw for group
		{"rwxrw-", 0o760, false},

		// Parse full permission specs as a string: Mean rw for owner,
		// rw for group and rwx for other.
		{"rw-rw-rwx", 0o667, false},

		// Invalid char should be an error
		{"rwSrw-rwx", 0o667, true},
	}
)

func TestFileMode(t *testing.T) {
	for _, tc := range fmTests {
		out, err := ParseFileMode(tc.in)
		if tc.is_err {
			assert.Error(t, err)
			continue
		}
		assert.NoError(t, err)
		assert.Equal(t, out, tc.out)
	}
}
