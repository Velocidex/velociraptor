/*

This code is based on the sigma-go project
https://github.com/bradleyjkemp/sigma-go

*/

package modifiers

import (
	"context"
	"fmt"
	"net"
	"regexp"
	"strings"

	"www.velocidex.com/golang/vfilter/types"
)

// ValueModifier modifies the expected values to return a modified
// set. The modifier may be used to expand the input set (e.g. the
// base64offset modifier will multiply the expected value to its
// different permutations). Alternativly, the modifier may replace the
// input values with rows of true or false depending on comparisons
// with the expected set.  Modifiers are chained together with the
// output of one feeding into the input of the next one. The overall
// match results in the boolean truthness of the final value set.

// NOTE: The design of the Sigma modifiers scheme is very inconsistant
// and it can not be generalized to a simple modifer pipeline. Only
// some modifiers can validly follow other modifiers. See the test
// file at sigma_test.go to check the valid set of modifier
// combinations in their supported order. Not every combination is
// supported or reasonable.
type ValueModifier interface {
	Modify(ctx context.Context, scope types.Scope,
		value []any, expected []any) (new_value []any, new_expected []any, err error)
}

var ValueModifiers = map[string]ValueModifier{
	"re":             AnyComparator{comp: re{}},
	"re_all":         AllComparator{comp: re{}},
	"windash":        windash{},
	"base64":         b64{},
	"base64offset":   b64offset{},
	"cidr":           AnyComparator{cidr{}},
	"wide":           wide{},
	"gt":             AnyComparator{gt{}},
	"gte":            AnyComparator{gte{}},
	"lt":             AnyComparator{lt{}},
	"lte":            AnyComparator{lte{}},
	"vql":            AnyComparator{vql{}},
	"endswith":       AnyComparator{endswith{}},
	"endswith_all":   AllComparator{endswith{}},
	"startswith":     AnyComparator{startswith{}},
	"startswith_all": AllComparator{startswith{}},
	"contains":       AnyComparator{contains{}},
	"contains_all":   AllComparator{contains{}},
	"all":            AllComparator{defaultModifier{}},
}

// The default modifier will return true if any of its values matches
// any of the expected set.
type defaultModifier struct{}

func (self defaultModifier) Matches(
	ctx context.Context, scope types.Scope,
	actual, expected any) (bool, error) {

	// Delegate actual comparisons to the scope.
	res := scope.Eq(actual, expected)
	if res {
		return res, nil
	}

	// Sometimes event logs have integers encoded as strings but the
	// detections use an integer so we need to special case it here
	// and try again comparing the stringified form of both values.
	return coerceString(actual) == coerceString(expected), nil
}

type contains struct{}

func (contains) Matches(
	ctx context.Context, scope types.Scope,
	actual, expected any) (bool, error) {

	actual_str := coerceString(actual)

	// If expected is a list it must be interpreted in OR
	// context. Expected can become a list after passing through
	// filters like base64offset.
	switch t := expected.(type) {
	case []string:
		for _, item := range t {
			if strings.Contains(actual_str, item) {
				return true, nil
			}
		}
	}

	// The Sigma spec defines that by default comparisons are case-insensitive
	return strings.Contains(
		strings.ToLower(actual_str),
		strings.ToLower(coerceString(expected))), nil
}

type endswith struct{}

func (endswith) Matches(
	ctx context.Context, scope types.Scope,
	actual, expected any) (bool, error) {
	// The Sigma spec defines that by default comparisons are case-insensitive
	return strings.HasSuffix(
		strings.ToLower(coerceString(actual)),
		strings.ToLower(coerceString(expected))), nil
}

type startswith struct{}

func (startswith) Matches(
	ctx context.Context, scope types.Scope,
	actual, expected any) (bool, error) {
	// The Sigma spec defines that by default comparisons are case-insensitive
	return strings.HasPrefix(
		strings.ToLower(coerceString(actual)),
		strings.ToLower(coerceString(expected))), nil
}

type windash struct{}

var cmdflagRegex = regexp.MustCompile("\\s-([a-zA-Z0-9])")

func (windash) Modify(ctx context.Context, scope types.Scope,
	value []any, expected []any) (new_value []any, new_expected []any, err error) {
	for _, e := range expected {
		expected_str := coerceString(e)
		new_expected = append(new_expected, []string{
			expected_str,
			cmdflagRegex.ReplaceAllString(coerceString(expected_str), " /$1"),
			cmdflagRegex.ReplaceAllString(coerceString(expected_str), " ―$1"),
			cmdflagRegex.ReplaceAllString(coerceString(expected_str), " —$1"),
			cmdflagRegex.ReplaceAllString(coerceString(expected_str), " –$1"),
		})
	}
	return value, new_expected, nil
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

func GetModifiers(modifiers []string) (res []ValueModifier, err error) {
	for i := 0; i < len(modifiers); i++ {
		modifier_name := modifiers[i]

		// "all" is not a real modifier it just influences some
		// previous modifiers. Detect "contains" followed by all and
		// handle it specially.
		switch modifier_name {

		// Only the following can have "all" follow
		case "contains", "re", "endswith", "startswith":
			if i+1 < len(modifiers) && modifiers[i+1] == "all" {
				modifier_name += "_all"
				i++
			}
		}

		// Special casing for all
		modifier, pres := ValueModifiers[modifier_name]
		if !pres {
			return nil, fmt.Errorf("unknown modifier %s", modifier_name)
		}
		res = append(res, modifier)
	}

	// If no modifiers are specified we use the default one.
	if len(res) == 0 {
		res = append(res, AnyComparator{defaultModifier{}})
	}

	return res, nil
}
