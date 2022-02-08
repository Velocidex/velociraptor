package constants

import (
	"context"
	"time"

	"www.velocidex.com/golang/velociraptor/constants"
	"www.velocidex.com/golang/velociraptor/utils"
	"www.velocidex.com/golang/vfilter"
	"www.velocidex.com/golang/vfilter/types"
)

func GetNTFSCacheTime(ctx context.Context, scope vfilter.Scope) time.Duration {
	cache_life := int64(0)
	cache_life_any, pres := scope.Resolve(constants.NTFS_CACHE_TIME)
	if pres {
		switch t := cache_life_any.(type) {
		case *vfilter.StoredExpression:
			cache_life_any = t.Reduce(ctx, scope)

		case types.LazyExpr:
			cache_life_any = t.Reduce(ctx)
		}

		cache_life, _ = utils.ToInt64(cache_life_any)
	}
	if cache_life == 0 {
		cache_life = 60
	} else {
		scope.Log("Will expire NTFS cache every %v sec\n", cache_life)
	}

	return time.Duration(cache_life) * time.Second
}
