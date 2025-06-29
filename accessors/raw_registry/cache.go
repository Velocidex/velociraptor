package raw_registry

import (
	"time"

	"github.com/Velocidex/ttlcache/v2"
	"www.velocidex.com/golang/velociraptor/constants"
	vql_subsystem "www.velocidex.com/golang/velociraptor/vql"
	"www.velocidex.com/golang/vfilter"
)

const (
	RAW_CACHE_TAG = "_RAW_REG_CACHE"
)

type RawRegFileSystemAccessorCache struct {
	lru         *ttlcache.Cache
	readdir_lru *ttlcache.Cache
}

func (self *RawRegFileSystemAccessorCache) Close() {
	self.lru.Close()
	self.readdir_lru.Close()
}

func getRegFileSystemAccessorCache(scope vfilter.Scope) *RawRegFileSystemAccessorCache {
	cache, ok := vql_subsystem.CacheGet(
		scope, RAW_CACHE_TAG).(*RawRegFileSystemAccessorCache)
	if ok {
		return cache
	}

	cache = &RawRegFileSystemAccessorCache{
		lru:         ttlcache.NewCache(),
		readdir_lru: ttlcache.NewCache(),
	}

	cache_size := int(vql_subsystem.GetIntFromRow(
		scope, scope, constants.RAW_REG_CACHE_SIZE))
	if cache_size == 0 {
		cache_size = 1000
	}

	cache_time := vql_subsystem.GetIntFromRow(
		scope, scope, constants.RAW_REG_CACHE_TIME)
	if cache_time == 0 {
		cache_time = 10
	}

	cache.lru.SetCacheSizeLimit(cache_size)
	_ = cache.lru.SetTTL(time.Second * time.Duration(cache_time))
	cache.lru.SkipTTLExtensionOnHit(true)

	cache.readdir_lru.SetCacheSizeLimit(cache_size)
	_ = cache.readdir_lru.SetTTL(time.Second * time.Duration(cache_time))
	cache.readdir_lru.SkipTTLExtensionOnHit(true)

	// Add the cache to the root scope so it can be visible outside
	// our scope. This should maximize cache hits
	root_scope := vql_subsystem.GetRootScope(scope)

	err := root_scope.AddDestructor(func() {
		cache.Close()
		cache.lru.Close()
		cache.readdir_lru.Close()
	})
	if err != nil {
		cache.Close()
		cache.lru.Close()
		cache.readdir_lru.Close()

		return cache
	}

	vql_subsystem.CacheSet(root_scope, RAW_CACHE_TAG, cache)

	return cache
}
