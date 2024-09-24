package utils

import (
	"fmt"
	"testing"

	"github.com/Velocidex/ordereddict"
	"www.velocidex.com/golang/velociraptor/vtesting/goldie"
)

func TestDictUtils(t *testing.T) {
	var nil_dict *ordereddict.Dict

	res := ordereddict.NewDict().
		Set("Inner", ordereddict.NewDict().
			Set("X", 1).
			Set("Y", "String")).
		Set("Inner2", "String").
		Set("Inner3", []string{"Foo"}).
		Set("NilInner", nil_dict).
		Set("Inner4", []interface{}{3, "X", nil})

	golden := ""
	for _, k := range []string{
		"NotExist",
		"Inner.X", // Really an int
		"Inner2",
		"Inner.Y",
		"Inner3",                  // Really an array so should not return string
		"Inner3.0",                // First index
		"Inner3.Foo",              // Not really a dict
		"NilInner.X",              // Should not crash!
		"Inner4.NotExist",         //
		"Inner4.0",                // Really an int
		"Inner4.1",                // A string.
		"Inner4.2",                // Nil
		"Inner4.10", "Inner4.-10", // out of bound index
	} {
		golden += fmt.Sprintf("%v -> '%v'\n", k, GetString(res, k))
	}

	goldie.Assert(t, "TestDictUtils", []byte(golden))
}
