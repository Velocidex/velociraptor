package utils

import (
	"testing"

	"github.com/Velocidex/ordereddict"
	"github.com/stretchr/testify/assert"
	"www.velocidex.com/golang/velociraptor/json"
	"www.velocidex.com/golang/velociraptor/vtesting/goldie"
)

// Sanitized strings should be prefectly reversible.
func TestSanitize(t *testing.T) {
	golden := ordereddict.NewDict()
	for _, name := range []string{
		"Simple string",
		"你好",
		"Word \"With\" quotes",
		"../../../",
		"foo.db",
		"bar.json.db",
		"filename:with:col.db",
		// Binary string with invalid utf8 sequence
		"\x00\x01\xf0\xf2\xff\xc3\x28",
		`\\.\C:\你好世界\"你好/世界.db"`,

		// Windows can not represent a name with a trailing .
		"foo.",

		"../../foo.",
	} {
		sanitized := SanitizeString(name)
		unsanitized := UnsanitizeComponent(sanitized)

		// fmt.Printf("Name %v (% x) -> %v -> (% x)\n", name, []byte(name), sanitized, []byte(unsanitized))
		assert.Equal(t, unsanitized, name)

		golden.Set(name, sanitized)
	}
	goldie.Assert(t, "TestSanitize", json.MustMarshalIndent(golden))
}

func TestSanitizeForZip(t *testing.T) {
	golden := ordereddict.NewDict()
	for _, name := range []string{
		"Simple string",
		"你好",
		"Word \"With\" quotes",
		"../../../",
		"foo.db",
		"bar.json.db",
		"filename:with:col.db",
		// Binary string with invalid utf8 sequence
		"\x00\x01\xf0\xf2\xff\xc3\x28",
		`\\.\C:\你好世界\"你好/世界.db"`,

		// Windows can not represent a name with a trailing .
		"foo.",
	} {
		sanitized := SanitizeStringForZip(name)
		unsanitized := UnsanitizeComponentForZip(sanitized)

		// fmt.Printf("Name %v (% x) -> %v -> (% x)\n", name, []byte(name), sanitized, []byte(unsanitized))
		assert.Equal(t, unsanitized, name)

		golden.Set(name, sanitized)
	}
	goldie.Assert(t, "TestSanitizeForZip", json.MustMarshalIndent(golden))
}
