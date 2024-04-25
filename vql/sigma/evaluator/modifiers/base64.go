package modifiers

import (
	"context"
	"encoding/base64"

	"www.velocidex.com/golang/vfilter/types"
)

var startOffsets = [3]int{0, 2, 3}
var endOffsets = [3]int{0, -3, -2}

func b64ShiftEncode(value string, shift int) string {
	endOffset := endOffsets[(len(value)+shift)%3]

	switch shift {
	case 0:
	case 1:
		value = " " + value
	case 2:
		value = "  " + value
	}

	encoded := base64.StdEncoding.EncodeToString([]byte(value))
	return encoded[startOffsets[shift]:(len(encoded) + endOffset)]
}

type b64 struct{}

func (b64) Modify(ctx context.Context, scope types.Scope,
	value []any, expected []any) (new_value []any, new_expected []any, err error) {
	for _, e := range expected {
		expected_str := coerceString(e)
		new_expected = append(new_expected,
			base64.StdEncoding.EncodeToString([]byte(expected_str)))
	}
	return value, new_expected, nil
}

type b64offset struct{}

func (b64offset) Modify(ctx context.Context, scope types.Scope,
	value []any, expected []any) (new_value []any, new_expected []any, err error) {

	for _, e := range expected {
		expected_str := coerceString(e)

		// Append expected to be a list of strings - this will always
		// be interpreted in OR context by following modifiers.
		new_expected = append(new_expected, []string{
			b64ShiftEncode(expected_str, 0),
			b64ShiftEncode(expected_str, 1),
			b64ShiftEncode(expected_str, 2)})
	}
	return value, new_expected, nil
}
