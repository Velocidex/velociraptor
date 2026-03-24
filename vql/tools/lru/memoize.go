package lru

import (
	"context"
	"sync"
	"time"

	"github.com/Velocidex/ordereddict"
	"www.velocidex.com/golang/velociraptor/utils"
	vql_subsystem "www.velocidex.com/golang/velociraptor/vql"
	vfilter "www.velocidex.com/golang/vfilter"
	"www.velocidex.com/golang/vfilter/arg_parser"
	"www.velocidex.com/golang/vfilter/types"
)

type _CacheObj struct {
	LRUCache

	mu         sync.Mutex
	query      vfilter.StoredQuery
	key_column string
	expires    time.Time
	period     time.Duration
	name       string

	ctx   context.Context
	scope vfilter.Scope
}

func (self *_CacheObj) Materialize() {
	self.mu.Lock()
	defer self.mu.Unlock()

	self.materialize()
}

func (self *_CacheObj) Get(key string) (vfilter.Any, bool) {
	self.mu.Lock()
	defer self.mu.Unlock()

	// Expire the entire cache when it gets too old.
	if time.Now().After(self.expires) {
		self.materialize()
	}

	value, pres := self.LRUCache.Get(key)
	return value, pres
}

func (self *_CacheObj) materialize() {
	self.LRUCache.Purge()

	count := 0
	for row_item := range self.query.Eval(self.ctx, self.scope) {
		key, pres := self.scope.Associative(row_item, self.key_column)
		if pres {
			key_str := utils.ToString(key)
			self.LRUCache.Set(key_str, row_item)
			count++
		}
	}

	self.scope.Log("memoize %v: Filled cache with %v rows", self.name, count)
	self.expires = time.Now().Add(self.period)
}

func NewCacheObj(ctx context.Context,
	scope vfilter.Scope, opts Options,
	name string,
	query vfilter.StoredQuery,
	key_column string,
	period time.Duration) (*_CacheObj, error) {
	res, err := NewLRU(ctx, scope, opts)
	if err != nil {
		return nil, err
	}

	return &_CacheObj{
		name:       name,
		LRUCache:   res,
		query:      query,
		key_column: key_column,
		period:     period,
		ctx:        ctx,
		scope:      scope,
	}, nil
}

type _MemoizeFunctionArgs struct {
	Query    types.StoredQuery `vfilter:"required,field=query,doc=Query to expand into memory"`
	Key      string            `vfilter:"required,field=key,doc=The name of the column to use as a key."`
	Period   int64             `vfilter:"optional,field=period,doc=The latest age of the cache."`
	Name     string            `vfilter:"optional,field=name,doc=The name of this cache."`
	Filename string            `vfilter:"optional,field=filename,doc=Filename for a persistant cache."`
}

type _MemoizeFunction struct{}

func (self _MemoizeFunction) Info(scope vfilter.Scope, type_map *vfilter.TypeMap) *vfilter.FunctionInfo {
	return &vfilter.FunctionInfo{
		Name:    "memoize",
		Doc:     "Memoize a query into memory.",
		ArgType: type_map.AddType(scope, &_MemoizeFunctionArgs{}),
		Version: 3,
	}
}

func (self _MemoizeFunction) Call(ctx context.Context, scope vfilter.Scope,
	args *ordereddict.Dict) vfilter.Any {

	arg := &_MemoizeFunctionArgs{}
	err := arg_parser.ExtractArgsWithContext(ctx, scope, args, arg)
	if err != nil {
		scope.Log("memoize: %s", err.Error())
		return vfilter.Null{}
	}

	if arg.Period == 0 {
		arg.Period = 60
	}

	period := time.Duration(arg.Period) * time.Second

	opts := Options{}
	opts.Filename = arg.Filename

	// Make sure that the underlying LRU does not expire anything - we
	// handle expiry ourselves.
	opts.MaxExpirySec = int(arg.Period) + 100
	opts.UpdateExpiryOnAccess = false
	opts.MaxSize = 1000000

	if arg.Name == "" {
		arg.Name = vfilter.FormatToString(scope, arg.Query)
	}

	// The _CacheObj will outlast this function call so we tie its
	// context to the scope.
	sub_ctx, cancel := context.WithCancel(context.Background())
	vql_subsystem.GetRootScope(scope).AddDestructor(cancel)

	res, err := NewCacheObj(sub_ctx, scope, opts,
		arg.Name, arg.Query, arg.Key, period)
	if err != nil {
		scope.Log("memoize: %s", err.Error())
		return vfilter.Null{}
	}

	return res
}

// Caches can be associative
type _CacheAssociative struct{}

func (self _CacheAssociative) Applicable(a vfilter.Any, b vfilter.Any) bool {
	_, ok := a.(*_CacheObj)
	return ok
}

// Associate object a with key b
func (self _CacheAssociative) Associative(
	scope vfilter.Scope,
	item vfilter.Any, key vfilter.Any) (vfilter.Any, bool) {
	cache_obj, ok := item.(*_CacheObj)
	if !ok {
		return vfilter.Null{}, false
	}

	lazy_key, ok := key.(types.LazyExpr)
	if ok {
		key = lazy_key.ReduceWithScope(cache_obj.ctx, scope)
	}

	return cache_obj.Get(utils.ToString(key))
}

func (self _CacheAssociative) GetMembers(scope vfilter.Scope, a vfilter.Any) []string {
	return nil
}

func init() {
	vql_subsystem.RegisterProtocol(&_CacheAssociative{})
	vql_subsystem.RegisterFunction(&_MemoizeFunction{})
}
