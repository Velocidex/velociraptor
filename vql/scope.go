/*
   Velociraptor - Dig Deeper
   Copyright (C) 2019-2025 Rapid7 Inc.

   This program is free software: you can redistribute it and/or modify
   it under the terms of the GNU Affero General Public License as published
   by the Free Software Foundation, either version 3 of the License, or
   (at your option) any later version.

   This program is distributed in the hope that it will be useful,
   but WITHOUT ANY WARRANTY; without even the implied warranty of
   MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
   GNU Affero General Public License for more details.

   You should have received a copy of the GNU Affero General Public License
   along with this program.  If not, see <https://www.gnu.org/licenses/>.
*/
// A cache which is stored in the VQL scope.
package vql

import (
	"sync"

	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	"www.velocidex.com/golang/velociraptor/constants"
	"www.velocidex.com/golang/velociraptor/utils"
	"www.velocidex.com/golang/vfilter"
)

const (
	CACHE_VAR = "$cache"
)

type ScopeCache struct {
	cache map[string]interface{}

	mu sync.Mutex
}

func (self *ScopeCache) Set(key string, value interface{}) {
	self.mu.Lock()
	defer self.mu.Unlock()
	self.cache[key] = value
}

func getCache(scope vfilter.Scope) *ScopeCache {
	any_obj, pres := scope.GetContext(CACHE_VAR)
	if !pres {
		return NewScopeCache()
	}
	cache, ok := any_obj.(*ScopeCache)
	if !ok {
		return NewScopeCache()
	}
	return cache
}

func CacheGet(scope vfilter.Scope, key string) interface{} {
	cache := getCache(scope)
	defer scope.SetContext(CACHE_VAR, cache)

	cache.mu.Lock()
	defer cache.mu.Unlock()

	return cache.cache[key]
}

func CacheSet(scope vfilter.Scope, key string, value interface{}) {
	cache := getCache(scope)
	defer scope.SetContext(CACHE_VAR, cache)

	cache.mu.Lock()
	defer cache.mu.Unlock()

	cache.cache[key] = value
}

// The server config is sensitive and so it is *not* stored in the
// scope vars and so can not be accessed by the VQL query
// directly. VQL plugins can access it via this method.
func GetServerConfig(scope vfilter.Scope) (*config_proto.Config, bool) {
	config_any := CacheGet(scope, constants.SCOPE_SERVER_CONFIG)
	if utils.IsNil(config_any) {
		return nil, false
	}
	config, ok := config_any.(*config_proto.Config)
	return config, ok
}

func NewScopeCache() *ScopeCache {
	return &ScopeCache{
		cache: make(map[string]interface{}),
	}
}
