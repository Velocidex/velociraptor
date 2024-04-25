package modifiers

import (
	"context"

	"www.velocidex.com/golang/vfilter/types"
)

type re struct{}

func (re) Matches(
	ctx context.Context, scope types.Scope,
	actual any, expected any) (bool, error) {

	// Delegate actual comparisons to the scope.
	return scope.Match(expected, actual), nil
}
