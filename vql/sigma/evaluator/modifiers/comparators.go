package modifiers

import (
	"context"

	"www.velocidex.com/golang/vfilter/types"
)

// A comparator is a simple way to compare two values using an
// operator. This is used by the AnyModifier and AllModifiers.

type Comparator interface {
	Matches(
		ctx context.Context, scope types.Scope,
		actual any, expected any) (bool, error)
}

// If any of the values compares to any of the expected set, then
// modify the value to a single row of true.
type AnyComparator struct {
	comp Comparator
}

// Match any of the expected set
func (self AnyComparator) modifyValue(
	ctx context.Context, scope types.Scope,
	value any, expected []any) (new_values []any, err error) {
	for _, e := range expected {
		matched, err := self.comp.Matches(ctx, scope, value, e)
		if err != nil {
			return nil, err
		}

		if matched {
			return []any{true}, nil
		}
	}

	// Nothing matched
	return nil, nil
}

func (self AnyComparator) Modify(
	ctx context.Context, scope types.Scope,
	value []any, expected []any) (res []any, new_expected []any, err error) {
	for _, v := range value {
		m, err := self.modifyValue(ctx, scope, v, expected)
		if err != nil {
			return nil, expected, err
		}

		res = append(res, m...)
	}
	return res, expected, nil
}

// Filter the values by those that match all the expected set.
type AllComparator struct {
	comp Comparator
}

// Match any of the expected set
func (self AllComparator) modifyValue(
	ctx context.Context, scope types.Scope,
	value any, expected []any) ([]any, error) {

	// No expected set, we did not match anything
	if len(expected) == 0 {
		return nil, nil
	}

	for _, e := range expected {
		matched, err := self.comp.Matches(ctx, scope, value, e)
		if err != nil {
			return nil, err
		}

		if !matched {
			return nil, nil
		}
	}

	// If we get here nothing was rejected - we have a match
	return []any{true}, nil
}

func (self AllComparator) Modify(
	ctx context.Context, scope types.Scope,
	value []any, expected []any) (res []any, new_expected []any, err error) {
	for _, v := range value {
		m, err := self.modifyValue(ctx, scope, v, expected)
		if err != nil {
			return nil, expected, err
		}

		res = append(res, m...)
	}
	return res, expected, nil
}
