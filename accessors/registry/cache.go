//go:build windows
// +build windows

package registry

import (
	"time"

	"github.com/Velocidex/ttlcache/v2"
	"www.velocidex.com/golang/velociraptor/constants"
	vql_subsystem "www.velocidex.com/golang/velociraptor/vql"
	"www.velocidex.com/golang/vfilter"
)

const (
	CACHE_TAG = "_REG_CACHE"
)

type RegFileSystemAccessorCache struct {
	lru         *ttlcache.Cache
	readdir_lru *ttlcache.Cache
}

func (self *RegFileSystemAccessorCache) GetDir(key string) (*readDirLRUItem, bool) {
	if self.readdir_lru == nil {
		return nil, false
	}

	cached, err := self.readdir_lru.Get(key)
	if err == nil {
		cached_res, ok := cached.(*readDirLRUItem)
		if ok {
			metricsReadDirLruHit.Inc()
			return cached_res, true
		}
	}
	metricsReadDirLruMiss.Inc()

	return nil, false
}

func (self *RegFileSystemAccessorCache) SetDir(key string, dir *readDirLRUItem) {
	if self.readdir_lru != nil {
		self.readdir_lru.Set(key, dir)
	}
}

func (self *RegFileSystemAccessorCache) Get(key string) (*RegKeyInfo, bool) {
	if self.lru == nil {
		return nil, false
	}

	cached, err := self.lru.Get(key)
	if err == nil {
		res, ok := cached.(*RegKeyInfo)
		if ok {
			metricsLruHit.Inc()
			return res, true
		}
	}

	metricsLruMiss.Inc()
	return nil, false
}

func (self *RegFileSystemAccessorCache) Set(key string, value *RegKeyInfo) {
	if self.lru != nil {
		self.lru.Set(key, value)
	}
}

func (self *RegFileSystemAccessorCache) Close() {
	self.lru.Close()
	self.readdir_lru.Close()
}

func getRegFileSystemAccessorCache(scope vfilter.Scope) *RegFileSystemAccessorCache {
	cache, ok := vql_subsystem.CacheGet(scope, CACHE_TAG).(*RegFileSystemAccessorCache)
	if ok {
		return cache
	}

	cache_size := int(vql_subsystem.GetIntFromRow(
		scope, scope, constants.REG_CACHE_SIZE))
	if cache_size == 0 {
		cache_size = 1000
	}

	// Cache is disabled.
	if cache_size < 0 {
		return &RegFileSystemAccessorCache{}
	}

	cache_time := vql_subsystem.GetIntFromRow(
		scope, scope, constants.REG_CACHE_TIME)
	if cache_time == 0 {
		cache_time = 10
	}

	cache = &RegFileSystemAccessorCache{
		lru:         ttlcache.NewCache(),
		readdir_lru: ttlcache.NewCache(),
	}

	cache.lru.SetCacheSizeLimit(cache_size)
	cache.lru.SetTTL(time.Second * time.Duration(cache_time))
	cache.lru.SkipTTLExtensionOnHit(true)

	cache.readdir_lru.SetCacheSizeLimit(cache_size)
	cache.readdir_lru.SetTTL(time.Second * time.Duration(cache_time))
	cache.readdir_lru.SkipTTLExtensionOnHit(true)

	// Add the cache to the root scope so it can be visible outside
	// our scope. This should maximize cache hits
	root_scope := vql_subsystem.GetRootScope(scope)

	root_scope.AddDestructor(func() {
		cache.Close()
		cache.lru.Close()
		cache.readdir_lru.Close()
	})
	vql_subsystem.CacheSet(root_scope, CACHE_TAG, cache)

	return cache
}
