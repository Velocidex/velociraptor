// A cache which is stored in the VQL scope.
package vql

import (
	"www.velocidex.com/golang/vfilter"
)

const (
	CACHE_VAR = "$cache"
)

type ScopeCache struct {
	cache map[string]interface{}
}

func CacheGet(scope *vfilter.Scope, key string) interface{} {
	any_obj, _ := scope.Resolve(CACHE_VAR)
	cache, ok := any_obj.(*ScopeCache)
	if ok {
		return cache.cache[key]
	}
	return nil
}

func CacheSet(scope *vfilter.Scope, key string, value interface{}) {
	any_obj, _ := scope.Resolve(CACHE_VAR)
	cache, ok := any_obj.(*ScopeCache)
	if ok {
		cache.cache[key] = value
	}
}

func NewScopeCache() *ScopeCache {
	return &ScopeCache{
		cache: make(map[string]interface{}),
	}
}
