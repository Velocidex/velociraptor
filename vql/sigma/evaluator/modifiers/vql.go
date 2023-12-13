package modifiers

import (
	"context"
	"errors"

	"www.velocidex.com/golang/velociraptor/utils"
	vql_subsystem "www.velocidex.com/golang/velociraptor/vql"
	"www.velocidex.com/golang/vfilter"
	"www.velocidex.com/golang/vfilter/types"
)

const (
	SIGMA_VQL_TAG = "_SIGMA_VQL"
)

type LambdaCache map[string]*vfilter.Lambda

type vql struct{}

func (self vql) Matches(
	ctx context.Context, scope types.Scope,
	actual any, expected any) (bool, error) {
	var err error

	expected_str, ok := expected.(string)
	if !ok {
		return false, errors.New("The `vql` modifier requires a lambda string")
	}

	lambda_cache_any := vql_subsystem.CacheGet(scope, SIGMA_VQL_TAG)
	if utils.IsNil(lambda_cache_any) {
		lambda_cache_any = make(LambdaCache)
		vql_subsystem.CacheSet(scope, SIGMA_VQL_TAG, lambda_cache_any)
	}

	lambda_cache, ok := lambda_cache_any.(LambdaCache)
	if !ok {
		return false, errors.New("LambdaCache is incorrect")
	}

	lambda, pres := lambda_cache[expected_str]
	if !pres {
		lambda, err = vfilter.ParseLambda(expected_str)
		if err != nil {
			return false, err
		}
		lambda_cache[expected_str] = lambda
	}
	return scope.Bool(
		lambda.Reduce(ctx, scope, []types.Any{actual})), nil
}
