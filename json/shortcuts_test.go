package json

import (
	"testing"

	"github.com/Velocidex/ordereddict"
	"www.velocidex.com/golang/velociraptor/vtesting/goldie"
)

func TestJsonlShortcuts(t *testing.T) {
	result := ordereddict.NewDict().
		Set("Simple", string(AppendJsonlItem([]byte("{\"foo\":1}\n"), "bar", 2))).
		Set("Nested", string(AppendJsonlItem([]byte("{\"foo\":1}\n"), "bar",
			ordereddict.NewDict().Set("F", 1).Set("B", 2)))).

		// Handle malformed JSON
		Set("Empty String", string(AppendJsonlItem([]byte(""), "bar", 2))).
		Set("Malformed", string(AppendJsonlItem([]byte("}"), "bar", 2))).
		Set("Malformed2", string(AppendJsonlItem([]byte("}\n"), "bar", 2)))
	goldie.Assert(t, "TestJsonlShortcuts", MustMarshalIndent(result))
}

func TestJsonFormat(t *testing.T) {
	obj := ordereddict.NewDict().Set("Foo", "Bar")
	subquery := Format(`{"Foo": %q}`, "Bar")

	query := Format(`{"a": %q, "b": %q, "integer": %q, "string": %q, "subquery": %s}`,
		obj, obj, 1, "hello", subquery)
	goldie.Assert(t, "TestJsonFormat", []byte(query))
}
