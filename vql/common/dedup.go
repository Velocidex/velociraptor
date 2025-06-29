package common

import (
	"context"
	"time"

	"github.com/Velocidex/ordereddict"
	"github.com/Velocidex/ttlcache/v2"
	"www.velocidex.com/golang/velociraptor/utils"
	vql_subsystem "www.velocidex.com/golang/velociraptor/vql"
	vfilter "www.velocidex.com/golang/vfilter"
	"www.velocidex.com/golang/vfilter/arg_parser"
)

type DedupPluginArgs struct {
	Key     string              `vfilter:"required,field=key,doc=A column name to use as dedup key."`
	Query   vfilter.StoredQuery `vfilter:"required,field=query,doc=Run this query to generate items."`
	Timeout uint64              `vfilter:"optional,field=timeout,doc=LRU expires in this much time (default 60 sec)."`
	Size    int64               `vfilter:"optional,field=size,doc=Size of the LRU cache."`
}

type DedupPlugin struct{}

func (self DedupPlugin) Call(
	ctx context.Context,
	scope vfilter.Scope,
	args *ordereddict.Dict) <-chan vfilter.Row {
	output_chan := make(chan vfilter.Row)

	go func() {
		defer close(output_chan)
		defer vql_subsystem.RegisterMonitor(ctx, "dedup", args)()

		arg := &DedupPluginArgs{}
		err := arg_parser.ExtractArgsWithContext(ctx, scope, args, arg)
		if err != nil {
			scope.Log("dedup: %v", err)
			return
		}

		if arg.Timeout == 0 {
			arg.Timeout = 60
		}

		if arg.Size == 0 {
			arg.Size = 1000
		}

		lru := ttlcache.NewCache()
		defer lru.Close()

		lru.SetCacheSizeLimit(int(arg.Size))
		_ = lru.SetTTL(time.Second * time.Duration(arg.Timeout))

		event_chan := arg.Query.Eval(ctx, scope)

		for {
			select {
			case <-ctx.Done():
				return

			case row, ok := <-event_chan:
				if !ok {
					return
				}

				key, pres := scope.Associative(row, arg.Key)
				if !pres {
					continue
				}

				key_str := utils.ToString(key)

				_, err := lru.Get(key_str)
				// Item is cached skip it
				if err == nil {
					continue
				}

				select {
				case <-ctx.Done():
					return

				case output_chan <- row:
					_ = lru.Set(key_str, true)
				}
			}
		}
	}()

	return output_chan
}

func (self DedupPlugin) Info(scope vfilter.Scope, type_map *vfilter.TypeMap) *vfilter.PluginInfo {
	return &vfilter.PluginInfo{
		Name:    "dedup",
		Doc:     "Dedups the query based on a column. This will suppress rows with identical values for the key column",
		ArgType: type_map.AddType(scope, &DedupPluginArgs{}),
	}
}

func init() {
	vql_subsystem.RegisterPlugin(&DedupPlugin{})
}
