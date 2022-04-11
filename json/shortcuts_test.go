package json

import (
	"testing"

	"github.com/Velocidex/ordereddict"
	"github.com/sebdah/goldie"
)

func TestJsonlShortcuts(t *testing.T) {
	result := ordereddict.NewDict().
		Set("Simple", AppendJsonlItem("{\"foo\":1}\n", "bar", 2)).
		Set("Nested", AppendJsonlItem("{\"foo\":1}\n", "bar",
			ordereddict.NewDict().Set("F", 1).Set("B", 2)))

	goldie.Assert(t, "TestJsonlShortcuts", MustMarshalIndent(result))
}
