package common

import (
	"context"
	"sync"
	"time"

	"github.com/Velocidex/ordereddict"
	"www.velocidex.com/golang/velociraptor/json"
	vql_subsystem "www.velocidex.com/golang/velociraptor/vql"
	vfilter "www.velocidex.com/golang/vfilter"
)

const (
	CACHE_KEY = "$cache_key"
)

type _CacheObj struct {
	mu         sync.Mutex
	expires    time.Time
	period     time.Duration
	expression vfilter.LazyExpr
	scope      *vfilter.Scope
	ctx        context.Context
	key        string
	cache      map[string]vfilter.Any
}

func (self *_CacheObj) Get(key string) (vfilter.Any, bool) {
	self.mu.Lock()
	defer self.mu.Unlock()

	// Expire the entire cache when it gets too old.
	if time.Now().After(self.expires) {
		self.cache = make(map[string]vfilter.Any)
		self.expires = time.Now().Add(self.period)
		return nil, false
	}

	value, pres := self.cache[key]
	return value, pres
}

func (self *_CacheObj) Set(key string, value vfilter.Any) {
	self.mu.Lock()
	defer self.mu.Unlock()

	self.cache[key] = value
}

func (self *_CacheObj) Materialize() {
	self.mu.Lock()
	defer self.mu.Unlock()

	self.scope.Log("Materializing memoized query")

	self.cache = make(map[string]vfilter.Any)
	stored_query := self.expression.ToStoredQuery(self.scope)
	for row_item := range stored_query.Eval(self.ctx, self.scope) {
		key, pres := self.scope.Associative(row_item, self.key)
		if pres {
			key_str := json.StringIndent(key)
			self.cache[key_str] = row_item
		}
	}
}

func NewCacheObj(ctx context.Context, scope *vfilter.Scope, key string) *_CacheObj {
	return &_CacheObj{
		scope: scope,
		ctx:   ctx,
		key:   key,
		cache: make(map[string]vfilter.Any),
	}
}

// Caches can be associative
type _CacheAssociative struct{}

func (self _CacheAssociative) Applicable(a vfilter.Any, b vfilter.Any) bool {
	_, ok := a.(*_CacheObj)
	return ok
}

// Associate object a with key b
func (self _CacheAssociative) Associative(
	scope *vfilter.Scope, a vfilter.Any, b vfilter.Any) (vfilter.Any, bool) {
	cache_obj, ok := a.(*_CacheObj)
	if !ok {
		return vfilter.Null{}, false
	}

	lazy_b, ok := b.(*vfilter.LazyExpr)
	if ok {
		b = lazy_b.ReduceWithScope(scope)
	}

	key := json.StringIndent(b)

	if time.Now().After(cache_obj.expires) {
		cache_obj.Materialize()
		cache_obj.expires = time.Now().Add(cache_obj.period)
	}

	res, pres := cache_obj.cache[key]
	if !pres {
		return vfilter.Null{}, false
	}
	return res, true
}

func (self _CacheAssociative) GetMembers(scope *vfilter.Scope, a vfilter.Any) []string {
	return nil
}

type _CacheFunctionArgs struct {
	Func   vfilter.LazyExpr `vfilter:"required,field=func,doc=A function to evaluate"`
	Name   string           `vfilter:"optional,field=name,doc=The global name of this cache (needed when more than one)"`
	Key    string           `vfilter:"required,field=key,doc=Cache key to use."`
	Period int64            `vfilter:"optional,field=period,doc=The latest age of the cache."`
}

type _CacheFunc struct{}

func (self _CacheFunc) Info(scope *vfilter.Scope, type_map *vfilter.TypeMap) *vfilter.FunctionInfo {
	return &vfilter.FunctionInfo{
		Name:    "cache",
		Doc:     "Creates a cache object",
		ArgType: type_map.AddType(scope, &_CacheFunctionArgs{}),
	}
}

func (self _CacheFunc) Call(ctx context.Context, scope *vfilter.Scope,
	args *ordereddict.Dict) vfilter.Any {

	arg := &_CacheFunctionArgs{}
	err := vfilter.ExtractArgs(scope, args, arg)
	if err != nil {
		scope.Log("cache: %s", err.Error())
		return vfilter.Null{}
	}

	if arg.Name == "" {
		arg.Name = CACHE_KEY
	}

	cache_obj := vql_subsystem.CacheGet(scope, arg.Name)
	if cache_obj == nil {
		if arg.Period == 0 {
			arg.Period = 60
		}

		new_cache_obj := NewCacheObj(ctx, scope, "")
		new_cache_obj.period = time.Duration(arg.Period) * time.Second
		cache_obj = new_cache_obj
	}
	defer vql_subsystem.CacheSet(scope, arg.Name, cache_obj)

	value, pres := cache_obj.(*_CacheObj).Get(arg.Key)
	if !pres {
		value = arg.Func.ReduceWithScope(scope)
		if !vql_subsystem.IsNull(value) {
			cache_obj.(*_CacheObj).Set(arg.Key, value)
		}
	}

	return value

}

type _MemoizeFunctionArgs struct {
	Query  vfilter.LazyExpr `vfilter:"required,field=query,doc=Query to expand into memory"`
	Key    string           `vfilter:"required,field=key,doc=The name of the column to use as a key."`
	Period int64            `vfilter:"optional,field=period,doc=The latest age of the cache."`
}

type _MemoizeFunction struct{}

func (self _MemoizeFunction) Info(scope *vfilter.Scope, type_map *vfilter.TypeMap) *vfilter.FunctionInfo {
	return &vfilter.FunctionInfo{
		Name:    "memoize",
		Doc:     "Memoize a query into memory.",
		ArgType: type_map.AddType(scope, &_MemoizeFunctionArgs{}),
	}
}

func (self _MemoizeFunction) Call(ctx context.Context, scope *vfilter.Scope,
	args *ordereddict.Dict) vfilter.Any {

	arg := &_MemoizeFunctionArgs{}
	err := vfilter.ExtractArgs(scope, args, arg)
	if err != nil {
		scope.Log("memoize: %s", err.Error())
		return vfilter.Null{}
	}

	if arg.Period == 0 {
		arg.Period = 60
	}

	result := NewCacheObj(ctx, scope, arg.Key)
	result.expression = arg.Query
	result.period = time.Duration(arg.Period) * time.Second

	return result
}

func init() {
	vql_subsystem.RegisterProtocol(&_CacheAssociative{})
	vql_subsystem.RegisterFunction(&_CacheFunc{})
	vql_subsystem.RegisterFunction(&_MemoizeFunction{})
}
