package modifiers

import (
	"context"

	"golang.org/x/text/encoding/unicode"
	"www.velocidex.com/golang/vfilter/types"
)

var (
	utf16Encoder = unicode.UTF16(unicode.LittleEndian, unicode.IgnoreBOM).NewEncoder()
)

type wide struct{}

func (wide) Modify(ctx context.Context, scope types.Scope,
	value []any, expected []any) (new_value []any, new_expected []any, err error) {

	for _, e := range expected {
		expected_str := coerceString(e)
		utf16, err := utf16Encoder.String(expected_str)
		if err == nil {
			new_expected = append(new_expected, utf16)
		}
	}
	return value, new_expected, nil
}
