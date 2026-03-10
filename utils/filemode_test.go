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

		// Parse as a short string
		{"x", 0o001, false},
		{"rwx", 0o007, false},

		// Parse full permission specs as a string
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
