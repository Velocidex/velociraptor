package modifiers

import (
	"context"
	"os"

	"www.velocidex.com/golang/velociraptor/utils"
	"www.velocidex.com/golang/velociraptor/vql/common"
	"www.velocidex.com/golang/vfilter/types"
)

type expandModifier struct{}

func (expandModifier) Modify(ctx context.Context, scope types.Scope,
	value []any, expected []any) (new_value []any, new_expected []any, err error) {

	for _, e := range expected {
		expected_str := coerceString(e)

		// Skip blocked env strings.
		if utils.InString(common.ShadowedEnv, expected_str) {
			continue
		}

		expected_str = os.Getenv(expected_str)
		if expected_str != "" {
			new_expected = append(new_expected, expected_str)
		}
	}
	return value, new_expected, nil
}
