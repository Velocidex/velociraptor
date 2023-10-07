package modifiers

import (
	"encoding/base64"
	"fmt"
	"net"
	"reflect"
	"regexp"
	"strings"

	"gopkg.in/yaml.v3"
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

	return func(actual, expected any) (bool, error) {
		var err error
		for _, modifier := range eventValueModifiers {
			actual, err = modifier.Modify(actual)
			if err != nil {
				return false, err
			}
		}
		for _, modifier := range valueModifiers {
			expected, err = modifier.Modify(expected)
			if err != nil {
				return false, err
			}
		}

		return comparator.Matches(actual, expected)
	}, nil
}

// Comparator defines how the comparison between actual and expected field values is performed (the default is exact string equality).
// For example, the `cidr` modifier uses a check based on the *net.IPNet Contains function
type Comparator interface {
	Matches(actual any, expected any) (bool, error)
}

type ComparatorFunc func(actual, expected any) (bool, error)

// ValueModifier modifies the expected value before it is passed to the comparator.
// For example, the `base64` modifier converts the expected value to base64.
type ValueModifier interface {
	Modify(value any) (any, error)
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
}

var ValueModifiers = map[string]ValueModifier{
	"base64": b64{},
}

// EventValueModifiers modify the value in the event before comparison (as opposed to ValueModifiers which modify the value in the rule)
var EventValueModifiers = map[string]ValueModifier{}

type baseComparator struct{}

func (baseComparator) Matches(actual, expected any) (bool, error) {
	switch {
	case actual == nil && expected == "null":
		// special case: "null" should match the case where a field isn't present (and so actual is nil)
		return true, nil
	default:
		// The Sigma spec defines that by default comparisons are case-insensitive
		return strings.EqualFold(coerceString(actual), coerceString(expected)), nil
	}
}

type contains struct{}

func (contains) Matches(actual, expected any) (bool, error) {
	// The Sigma spec defines that by default comparisons are case-insensitive
	return strings.Contains(strings.ToLower(coerceString(actual)), strings.ToLower(coerceString(expected))), nil
}

type endswith struct{}

func (endswith) Matches(actual, expected any) (bool, error) {
	// The Sigma spec defines that by default comparisons are case-insensitive
	return strings.HasSuffix(strings.ToLower(coerceString(actual)), strings.ToLower(coerceString(expected))), nil
}

type startswith struct{}

func (startswith) Matches(actual, expected any) (bool, error) {
	// The Sigma spec defines that by default comparisons are case-insensitive
	return strings.HasPrefix(strings.ToLower(coerceString(actual)), strings.ToLower(coerceString(expected))), nil
}

type containsCS struct{}

func (containsCS) Matches(actual, expected any) (bool, error) {
	return strings.Contains(coerceString(actual), coerceString(expected)), nil
}

type endswithCS struct{}

func (endswithCS) Matches(actual, expected any) (bool, error) {
	return strings.HasSuffix(coerceString(actual), coerceString(expected)), nil
}

type startswithCS struct{}

func (startswithCS) Matches(actual, expected any) (bool, error) {
	return strings.HasPrefix(coerceString(actual), coerceString(expected)), nil
}

type b64 struct{}

func (b64) Modify(value any) (any, error) {
	return base64.StdEncoding.EncodeToString([]byte(coerceString(value))), nil
}

type re struct{}

func (re) Matches(actual any, expected any) (bool, error) {
	re, err := regexp.Compile("(?i)" + coerceString(expected))
	if err != nil {
		return false, err
	}

	return re.MatchString(coerceString(actual)), nil
}

type cidr struct{}

func (cidr) Matches(actual any, expected any) (bool, error) {
	_, cidr, err := net.ParseCIDR(coerceString(expected))
	if err != nil {
		return false, err
	}

	ip := net.ParseIP(coerceString(actual))
	return cidr.Contains(ip), nil
}

type gt struct{}

func (gt) Matches(actual any, expected any) (bool, error) {
	gt, _, _, _, err := compareNumeric(actual, expected)
	return gt, err
}

type gte struct{}

func (gte) Matches(actual any, expected any) (bool, error) {
	_, gte, _, _, err := compareNumeric(actual, expected)
	return gte, err
}

type lt struct{}

func (lt) Matches(actual any, expected any) (bool, error) {
	_, _, lt, _, err := compareNumeric(actual, expected)
	return lt, err
}

type lte struct{}

func (lte) Matches(actual any, expected any) (bool, error) {
	_, _, _, lte, err := compareNumeric(actual, expected)
	return lte, err
}

func coerceString(v interface{}) string {
	switch vv := v.(type) {
	case string:
		return vv
	case []byte:
		return string(vv)
	default:
		return fmt.Sprint(vv)
	}
}

// coerceNumeric makes both operands into the widest possible number of the same type
func coerceNumeric(left, right interface{}) (interface{}, interface{}, error) {
	leftV := reflect.ValueOf(left)
	leftType := reflect.ValueOf(left).Type()
	rightV := reflect.ValueOf(right)
	rightType := reflect.ValueOf(right).Type()

	switch {
	// Both integers or both floats? Return directly
	case leftType.Kind() == reflect.Int && rightType.Kind() == reflect.Int:
		fallthrough
	case leftType.Kind() == reflect.Float64 && rightType.Kind() == reflect.Float64:
		return left, right, nil

	// Mixed integer, float? Return two floats
	case leftType.Kind() == reflect.Int && rightType.Kind() == reflect.Float64:
		fallthrough
	case leftType.Kind() == reflect.Float64 && rightType.Kind() == reflect.Int:
		floatType := reflect.TypeOf(float64(0))
		return leftV.Convert(floatType).Interface(), rightV.Convert(floatType).Interface(), nil

	// One or more strings? Parse and recurse.
	// We use `yaml.Unmarshal` to parse the string because it's a cheat's way of parsing either an integer or a float
	case leftType.Kind() == reflect.String:
		var leftParsed interface{}
		if err := yaml.Unmarshal([]byte(left.(string)), &leftParsed); err != nil {
			return nil, nil, err
		}
		return coerceNumeric(leftParsed, right)
	case rightType.Kind() == reflect.String:
		var rightParsed interface{}
		if err := yaml.Unmarshal([]byte(right.(string)), &rightParsed); err != nil {
			return nil, nil, err
		}
		return coerceNumeric(left, rightParsed)

	default:
		return nil, nil, fmt.Errorf("cannot coerce %T and %T to numeric", left, right)
	}
}

func compareNumeric(left, right interface{}) (gt, gte, lt, lte bool, err error) {
	left, right, err = coerceNumeric(left, right)
	if err != nil {
		return
	}

	switch left.(type) {
	case int:
		left := left.(int)
		right := right.(int)
		return left > right, left >= right, left < right, left <= right, nil
	case float64:
		left := left.(float64)
		right := right.(float64)
		return left > right, left >= right, left < right, left <= right, nil
	default:
		err = fmt.Errorf("internal, please report! coerceNumeric returned unexpected types %T and %T", left, right)
		return
	}
}
