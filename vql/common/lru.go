package common

import (
	"context"

	"github.com/Velocidex/ordereddict"
	"github.com/Velocidex/ttlcache/v2"
	vql_subsystem "www.velocidex.com/golang/velociraptor/vql"
	vfilter "www.velocidex.com/golang/vfilter"
	"www.velocidex.com/golang/vfilter/arg_parser"
)

type LRUCache struct {
	lru *ttlcache.Cache
}

// Setter protocol allows VQL set() to be used
func (self *LRUCache) Set(key string, value interface{}) {
	self.lru.Set(key, value)
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

	if arg.Size <= 0 {
		arg.Size = 1000
	}
	result.lru.SetCacheSizeLimit(int(arg.Size))

	return result
}

func init() {
	vql_subsystem.RegisterFunction(&LRUFunction{})
	vql_subsystem.RegisterProtocol(&LRUFunction{})
}
