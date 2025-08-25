package common

import (
	"context"

	"github.com/Velocidex/ordereddict"
	"github.com/Velocidex/ttlcache/v2"
	vql_subsystem "www.velocidex.com/golang/velociraptor/vql"
	vfilter "www.velocidex.com/golang/vfilter"
	"www.velocidex.com/golang/vfilter/arg_parser"
	"www.velocidex.com/golang/vfilter/types"
)

type LRUCache struct {
	lru *ttlcache.Cache
}

// Setter protocol allows VQL set() to be used
func (self *LRUCache) Set(key string, value interface{}) {
	_ = self.lru.Set(key, value)
}

func (self *LRUCache) Len() int {
	return int(self.lru.GetMetrics().Size)
}

func (self *LRUCache) Dump() *ordereddict.Dict {
	res := ordereddict.NewDict()
	for _, key := range self.lru.GetKeys() {
		v, err := self.lru.Get(key)
		if err == nil {
			res.Set(key, v)
		}
	}
	return res
}

type LRUFunctionArgs struct {
	Size int64 `vfilter:"optional,field=size,doc=Size of the LRU (default 1000)"`
}

type LRUFunction struct{}

// Associative protocol allows get to be used as well as "." operator.
func (self LRUFunction) Applicable(a vfilter.Any, b vfilter.Any) bool {
	_, a_ok := a.(*LRUCache)
	if !a_ok {
		return false
	}

	_, b_ok := b.(string)
	if !b_ok {
		return false
	}

	return true
}

func (self LRUFunction) Associative(
	scope vfilter.Scope, a vfilter.Any, b vfilter.Any) (
	vfilter.Any, bool) {

	cache, a_ok := a.(*LRUCache)
	if !a_ok {
		return vfilter.Null{}, false
	}

	key, b_ok := b.(string)
	if !b_ok {
		return vfilter.Null{}, false
	}

	// Support accessing some cache methods.
	switch b {
	case "@Dump":
		return cache.Dump(), true
	case "@Len":
		return cache.Len(), true
	case "@Metrics":
		return cache.lru.GetMetrics(), true
	}

	res, err := cache.lru.Get(key)
	if err == nil {
		return res, true
	}

	return vfilter.Null{}, false
}

func (self LRUFunction) GetMembers(
	scope vfilter.Scope, a vfilter.Any) []string {
	return nil
}

func (self LRUFunction) Info(scope vfilter.Scope, type_map *vfilter.TypeMap) *vfilter.FunctionInfo {
	return &vfilter.FunctionInfo{
		Name:    "lru",
		Doc:     "Creates an LRU object",
		ArgType: type_map.AddType(scope, &LRUFunctionArgs{}),
	}
}

func (self LRUFunction) Call(ctx context.Context, scope vfilter.Scope,
	args *ordereddict.Dict) vfilter.Any {

	arg := &LRUFunctionArgs{}
	err := arg_parser.ExtractArgsWithContext(ctx, scope, args, arg)
	if err != nil {
		scope.Log("lru: %s", err.Error())
		return vfilter.Null{}
	}

	result := &LRUCache{
		lru: ttlcache.NewCache(),
	}
	err = scope.AddDestructor(func() {
		result.lru.Close()
	})
	if err != nil {
		result.lru.Close()
		scope.Log("lru: %s", err.Error())
		return vfilter.Null{}
	}

	if arg.Size <= 0 {
		arg.Size = 1000
	}
	result.lru.SetCacheSizeLimit(int(arg.Size))

	return result
}

type _LRUBoolProtocol struct{}

func (self _LRUBoolProtocol) Applicable(a types.Any) bool {
	_, a_ok := a.(*LRUCache)
	return a_ok
}

func (self _LRUBoolProtocol) Bool(ctx context.Context, scope types.Scope,
	a types.Any) bool {
	lru, a_ok := a.(*LRUCache)
	if !a_ok {
		return false
	}

	return lru.Len() > 0
}

func init() {
	vql_subsystem.RegisterFunction(&LRUFunction{})
	vql_subsystem.RegisterProtocol(&LRUFunction{})
	vql_subsystem.RegisterProtocol(&_LRUBoolProtocol{})
}
