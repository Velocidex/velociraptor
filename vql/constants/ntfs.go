package constants

import (
	"context"
	"time"

	"www.velocidex.com/golang/velociraptor/constants"
	vql_subsystem "www.velocidex.com/golang/velociraptor/vql"
	"www.velocidex.com/golang/vfilter"
)

func GetNTFSCacheTime(ctx context.Context, scope vfilter.Scope) time.Duration {
	cache_life := vql_subsystem.GetIntFromRow(
		scope, scope, constants.NTFS_CACHE_TIME)
	if cache_life == 0 {
		cache_life = 600
	}

	res := time.Duration(cache_life) * time.Second
	return res
}
