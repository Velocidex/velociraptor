/*
   Velociraptor - Hunting Evil
   Copyright (C) 2019 Velocidex Innovations.

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
