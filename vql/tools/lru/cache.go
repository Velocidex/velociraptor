package lru

import (
	"context"

	"github.com/Velocidex/ordereddict"
	"www.velocidex.com/golang/velociraptor/acls"
	"www.velocidex.com/golang/velociraptor/utils"
	"www.velocidex.com/golang/velociraptor/vql"
	vql_subsystem "www.velocidex.com/golang/velociraptor/vql"
	vfilter "www.velocidex.com/golang/vfilter"
	"www.velocidex.com/golang/vfilter/arg_parser"
	"www.velocidex.com/golang/vfilter/types"
)

const (
	CACHE_KEY = "$cache_key"
)

type _LRUObj struct {
	LRUCache

	lambda *vfilter.Lambda
}

func NewLRU(ctx context.Context, scope vfilter.Scope, opts Options) (LRUCache, error) {
	if opts.Filename == "" {
		return NewMemoryCache(opts), nil
	}

	res, err := NewDiskCache(ctx, opts)
	if err != nil {
		return nil, err
	}
	return res, nil
}

type _CacheFunctionArgs struct {
	Func     types.LazyExpr  `vfilter:"optional,field=func,doc=A function to evaluate (deprecated - use a lambda instead)"`
	Lambda   *vfilter.Lambda `vfilter:"optional,field=lambda,doc=A VQL lambda to evaluate with the key as parameter. eg. x=>x+1 "`
	Name     string          `vfilter:"optional,field=name,doc=The global name of this cache (needed when more than one)"`
	Key      types.Any       `vfilter:"optional,field=key,doc=Cache key to use."`
	Period   int64           `vfilter:"optional,field=period,doc=The latest age of the cache."`
	Filename string          `vfilter:"optional,field=filename,doc=Filename for a persistent cache."`
	MaxSize  uint64          `vfilter:"optional,field=max_size,doc=Maximum size of the LRU (default 10000)."`
}

type _CacheFunc struct{}

func (self _CacheFunc) Info(scope vfilter.Scope, type_map *vfilter.TypeMap) *vfilter.FunctionInfo {
	return &vfilter.FunctionInfo{
		Name:     "cache",
		Doc:      "Creates a cache object",
		ArgType:  type_map.AddType(scope, &_CacheFunctionArgs{}),
		Metadata: vql.VQLMetadata().Permissions(acls.FILESYSTEM_WRITE).Build(),
		Version:  2,
	}
}

func (self _CacheFunc) Call(ctx context.Context, scope vfilter.Scope,
	args *ordereddict.Dict) vfilter.Any {

	arg := &_CacheFunctionArgs{}
	err := arg_parser.ExtractArgsWithContext(ctx, scope, args, arg)
	if err != nil {
		scope.Log("cache: %s", err.Error())
		return vfilter.Null{}
	}

	cache_key := arg.Name
	if cache_key == "" {
		cache_key = CACHE_KEY + arg.Name
	}

	cache_obj := vql_subsystem.CacheGet(scope, cache_key)
	if cache_obj == nil {
		if vql_subsystem.IsNull(arg.Func) && vql_subsystem.IsNull(arg.Lambda) {
			scope.Log("cache: One of `func` or `lambda` is required")
			return vfilter.Null{}
		}

		if arg.Period == 0 {
			arg.Period = 60
		}

		opts := Options{}
		opts.Filename = arg.Filename
		opts.MaxExpirySec = int(arg.Period)
		opts.UpdateExpiryOnAccess = true
		opts.MaxSize = int(arg.MaxSize)
		if opts.MaxSize == 0 {
			opts.MaxSize = 10000
		}

		new_lru, err := NewLRU(ctx, scope, opts)
		if err != nil {
			scope.Log("cache: %s", err.Error())
			return vfilter.Null{}
		}
		_ = vql_subsystem.GetRootScope(scope).AddDestructor(new_lru.Close)

		cache_obj = &_LRUObj{
			LRUCache: new_lru,
			lambda:   arg.Lambda,
		}
	}
	defer vql_subsystem.CacheSet(scope, cache_key, cache_obj)

	// We dont have to return anything if there is no key, just create
	// the cache object.
	if arg.Key == nil {
		return vfilter.Null{}
	}

	key := vql_subsystem.Materialize(ctx, scope, arg.Key)
	key_str := utils.ToString(key)

	lru, ok := cache_obj.(*_LRUObj)
	if !ok {
		scope.Log("cache: Something went wrong!")
		return vfilter.Null{}
	}

	value, pres := lru.Get(key_str)
	if !pres {
		if lru.lambda == nil {
			if arg.Func == nil {
				scope.Log("cache: Use lambda to cache the function")
				return vfilter.Null{}
			}
			value = arg.Func.ReduceWithScope(ctx, scope)

		} else {
			value = lru.lambda.Reduce(ctx, scope, []types.Any{key})
		}

		if !vql_subsystem.IsNull(value) {
			lru.Set(key_str, value)
		}
	}

	return value
}

func init() {
	vql_subsystem.RegisterFunction(&_CacheFunc{})
}
