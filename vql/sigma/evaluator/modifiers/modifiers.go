package modifiers

import (
	"context"
	"encoding/base64"
	"fmt"
	"net"
	"strings"

	"www.velocidex.com/golang/vfilter/types"
)

func GetComparator(modifiers ...string) (ComparatorFunc, error) {
	return getComparator(Comparators, modifiers...)
}

func GetComparatorCaseSensitive(modifiers ...string) (ComparatorFunc, error) {
	return getComparator(ComparatorsCaseSensitive, modifiers...)
}

func getComparator(comparators map[string]Comparator, modifiers ...string) (ComparatorFunc, error) {
	if len(modifiers) == 0 {
		return baseComparator{}.Matches, nil
	}

	// A valid sequence of modifiers is ([ValueModifier]*)[Comparator]?
	// If a comparator is specified, it must be in the last position and cannot be succeeded by any other modifiers
	// If no comparator is specified, the default comparator is used
	// TODO: The explanation above is incorrect (see https://github.com/SigmaHQ/pySigma/blob/main/sigma/modifiers.py)
	var valueModifiers []ValueModifier
	var eventValueModifiers []ValueModifier
	var comparator Comparator
	for i, modifier := range modifiers {
		comparatorModifier := comparators[modifier]
		valueModifier := ValueModifiers[modifier]
		eventValueModifier := EventValueModifiers[modifier]
		switch {
		// Validate correctness
		case comparatorModifier == nil && valueModifier == nil && eventValueModifier == nil:
			return nil, fmt.Errorf("unknown modifier %s", modifier)

		case i < len(modifiers)-1 && comparators[modifier] != nil:
			return nil, fmt.Errorf("comparator modifier %s must be the last modifier", modifier)

		// Build up list of modifiers
		case valueModifier != nil:
			valueModifiers = append(valueModifiers, valueModifier)

		case eventValueModifier != nil:
			eventValueModifiers = append(eventValueModifiers, eventValueModifier)

		case comparatorModifier != nil:
			comparator = comparatorModifier
		}
	}
	if comparator == nil {
		comparator = baseComparator{}
	}

	return func(
		ctx context.Context, scope types.Scope,
		actual, expected any) (bool, error) {
		expectedArr := []any{expected}
		var err error
		for _, modifier := range eventValueModifiers {
			actual, err = modifier.Modify(actual)
			if err != nil {
				return false, err
			}
		}
		for _, modifier := range valueModifiers {
			newExpectedMod := []any{}
			for _, expected := range expectedArr {
				newExpected, err := modifier.Modify(expected)
				if err != nil {
					return false, err
				}
				newExpectedMod = append(newExpectedMod, newExpected...)
			}
			expectedArr = newExpectedMod
		}
		for _, expected := range expectedArr {
			hasMatch, err := comparator.Matches(
				ctx, scope, actual, expected)
			if err != nil {
				return false, err
			}
			if hasMatch {
				return true, nil
			}
		}
		return false, nil
	}, nil
}

// Comparator defines how the comparison between actual and expected
// field values is performed (the default is exact string equality).
// For example, the `cidr` modifier uses a check based on the
// *net.IPNet Contains function
type Comparator interface {
	Matches(ctx context.Context, scope types.Scope,
		actual any, expected any) (bool, error)
}

type ComparatorFunc func(
	ctx context.Context, scope types.Scope,
	actual, expected any) (bool, error)

// ValueModifier modifies the expected value before it is passed to the comparator.
// For example, the `base64` modifier converts the expected value to base64.
type ValueModifier interface {
	Modify(value any) ([]any, error)
}

var Comparators = map[string]Comparator{
	"contains":   contains{},
	"endswith":   endswith{},
	"startswith": startswith{},
	"re":         re{},
	"cidr":       cidr{},
	"gt":         gt{},
	"gte":        gte{},
	"lt":         lt{},
	"lte":        lte{},
	"vql":        vql{},
}

var ComparatorsCaseSensitive = map[string]Comparator{
	"contains":   containsCS{},
	"endswith":   endswithCS{},
	"startswith": startswithCS{},
	"re":         re{},
	"cidr":       cidr{},
	"gt":         gt{},
	"gte":        gte{},
	"lt":         lt{},
	"lte":        lte{},
	"vql":        vql{},
}

var ValueModifiers = map[string]ValueModifier{
	"base64":       b64{},
	"base64offset": b64offset{},
}

// EventValueModifiers modify the value in the event before comparison
// (as opposed to ValueModifiers which modify the value in the rule)
var EventValueModifiers = map[string]ValueModifier{}

type baseComparator struct{}

func (baseComparator) Matches(
	ctx context.Context, scope types.Scope,
	actual, expected any) (bool, error) {

	// Delegate actual comparisons to the scope.
	return scope.Eq(actual, expected), nil
}

type contains struct{}

func (contains) Matches(
	ctx context.Context, scope types.Scope,
	actual, expected any) (bool, error) {
	// The Sigma spec defines that by default comparisons are case-insensitive
	return strings.Contains(strings.ToLower(coerceString(actual)), strings.ToLower(coerceString(expected))), nil
}

type endswith struct{}

func (endswith) Matches(
	ctx context.Context, scope types.Scope,
	actual, expected any) (bool, error) {
	// The Sigma spec defines that by default comparisons are case-insensitive
	return strings.HasSuffix(strings.ToLower(coerceString(actual)), strings.ToLower(coerceString(expected))), nil
}

type startswith struct{}

func (startswith) Matches(
	ctx context.Context, scope types.Scope,
	actual, expected any) (bool, error) {
	// The Sigma spec defines that by default comparisons are case-insensitive
	return strings.HasPrefix(strings.ToLower(coerceString(actual)), strings.ToLower(coerceString(expected))), nil
}

type containsCS struct{}

func (containsCS) Matches(
	ctx context.Context, scope types.Scope,
	actual, expected any) (bool, error) {
	return strings.Contains(coerceString(actual), coerceString(expected)), nil
}

type endswithCS struct{}

func (endswithCS) Matches(
	ctx context.Context, scope types.Scope,
	actual, expected any) (bool, error) {
	return strings.HasSuffix(coerceString(actual), coerceString(expected)), nil
}

type startswithCS struct{}

func (startswithCS) Matches(
	ctx context.Context, scope types.Scope,
	actual, expected any) (bool, error) {
	return strings.HasPrefix(coerceString(actual), coerceString(expected)), nil
}

var startOffsets = [3]int{0, 2, 3}
var endOffsets = [3]int{0, -3, -2}

func b64ShiftEncode(value any, shift int) any {
	valueStr := coerceString(value)
	endOffset := endOffsets[(len(valueStr)+shift)%3]
	encoded := base64.StdEncoding.EncodeToString([]byte(strings.Repeat(" ", shift) + valueStr))
	return encoded[startOffsets[shift]:(len(encoded) + endOffset)]
}

type b64 struct{}

func (b64) Modify(value any) ([]any, error) {
	return []any{base64.StdEncoding.EncodeToString([]byte(coerceString(value)))}, nil
}

type b64offset struct{}

func (b64offset) Modify(value any) ([]any, error) {
	return []any{
		b64ShiftEncode(value, 0),
		b64ShiftEncode(value, 1),
		b64ShiftEncode(value, 2),
	}, nil
}

type re struct{}

func (re) Matches(
	ctx context.Context, scope types.Scope,
	actual any, expected any) (bool, error) {

	// Delegate actual comparisons to the scope.
	return scope.Match(expected, actual), nil
}

type cidr struct{}

func (cidr) Matches(
	ctx context.Context, scope types.Scope,
	actual any, expected any) (bool, error) {
	_, cidr, err := net.ParseCIDR(coerceString(expected))
	if err != nil {
		return false, err
	}

	ip := net.ParseIP(coerceString(actual))
	return cidr.Contains(ip), nil
}

type gt struct{}

func (gt) Matches(
	ctx context.Context, scope types.Scope,
	actual any, expected any) (bool, error) {

	return scope.Gt(actual, expected), nil
}

type gte struct{}

func (gte) Matches(
	ctx context.Context, scope types.Scope,
	actual any, expected any) (bool, error) {

	return scope.Gt(actual, expected) ||
		scope.Eq(actual, expected), nil
}

type lt struct{}

func (lt) Matches(
	ctx context.Context, scope types.Scope,
	actual any, expected any) (bool, error) {

	return scope.Lt(actual, expected), nil
}

type lte struct{}

func (lte) Matches(
	ctx context.Context, scope types.Scope,
	actual any, expected any) (bool, error) {
	return scope.Lt(actual, expected) ||
		scope.Eq(actual, expected), nil
}

func coerceString(in interface{}) string {
	switch t := in.(type) {
	case string:
		return t
	case *string:
		return *t
	default:
		return fmt.Sprintf("%v", in)
	}
}
