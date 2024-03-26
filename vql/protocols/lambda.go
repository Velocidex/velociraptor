// Registers VQL Lambda functions as parameters

package protocols

import (
	"context"
	"errors"
	"fmt"

	"github.com/Velocidex/ordereddict"
	"www.velocidex.com/golang/velociraptor/utils"
	vql_subsystem "www.velocidex.com/golang/velociraptor/vql"
	"www.velocidex.com/golang/vfilter"
	"www.velocidex.com/golang/vfilter/arg_parser"
	"www.velocidex.com/golang/vfilter/types"
)

var (
	invalidLambdaError = errors.New("VQL lambda functions need to be a string of the form \"x=>...\"")
	lambdaCacheTag     = "_LAMBDA"
)

type lambdaCache map[string]*vfilter.Lambda

// Currently a lambda function needs to be a string.
func ParseLambda(ctx context.Context, scope types.Scope, args *ordereddict.Dict,
	value interface{}) (interface{}, error) {

	switch t := value.(type) {

	// Its already a lambda just return it.
	case *vfilter.Lambda:
		return t, nil

	case types.LazyExpr:
		return ParseLambda(ctx, scope, args, t.Reduce(ctx))

	case string:
		// Try to get precompiled lambda from cache.
		cached := getCache(scope)
		res, pres := cached[t]
		if pres {
			return res, nil
		}

		// Compile the lambda into the cache.
		compiled, err := vfilter.ParseLambda(t)
		if err != nil {
			return nil, err
		}

		cached[t] = compiled
		vql_subsystem.CacheSet(scope, lambdaCacheTag, cached)

		return compiled, nil

	default:
		return nil, fmt.Errorf("Got field type %T: %w", value, invalidLambdaError)
	}
}

func getCache(scope vfilter.Scope) lambdaCache {
	cached_any := vql_subsystem.CacheGet(scope, lambdaCacheTag)
	if utils.IsNil(cached_any) {
		return make(lambdaCache)
	}
	cached, ok := cached_any.(lambdaCache)
	if ok {
		return cached
	}
	return make(lambdaCache)
}

func init() {
	arg_parser.RegisterParser(&vfilter.Lambda{}, ParseLambda)
}
