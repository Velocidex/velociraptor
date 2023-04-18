/*
   Velociraptor - Dig Deeper
   Copyright (C) 2019-2022 Rapid7 Inc.

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
package functions

import (
	"context"
	"fmt"
	"reflect"
	"strings"

	"github.com/Velocidex/ordereddict"
	"github.com/Velocidex/ttlcache/v2"
	"www.velocidex.com/golang/velociraptor/logging"
	"www.velocidex.com/golang/velociraptor/utils"
	vql_subsystem "www.velocidex.com/golang/velociraptor/vql"
	"www.velocidex.com/golang/vfilter"
	"www.velocidex.com/golang/vfilter/arg_parser"
)

const (
	LOG_TAG = "last_log"
)

type logCacheEntry struct {
	last_time int64
}

type logCache struct {
	lru *ttlcache.Cache // map[string]logCacheEntry
}

type LogFunctionArgs struct {
	Message   string      `vfilter:"required,field=message,doc=Message to log."`
	DedupTime int64       `vfilter:"optional,field=dedup,doc=Suppress same message in this many seconds (default 60 sec)."`
	Args      vfilter.Any `vfilter:"optional,field=args,doc=An array of elements to apply into the format string."`
	Level     string      `vfilter:"optional,field=level,doc=Level to log at (DEFAULT, WARN, ERROR, INFO)."`
}

type LogFunction struct{}

func (self *LogFunction) Call(ctx context.Context,
	scope vfilter.Scope,
	args *ordereddict.Dict) vfilter.Any {
	arg := &LogFunctionArgs{}
	err := arg_parser.ExtractArgsWithContext(ctx, scope, args, arg)
	if err != nil {
		scope.Log("log: %s", err.Error())
		return false
	}

	if arg.DedupTime == 0 {
		arg.DedupTime = 60
	}

	// Get the log cache and check if the message was emitted recently.
	var log_cache *logCache

	log_cache_any := vql_subsystem.CacheGet(scope, LOG_TAG)
	if utils.IsNil(log_cache_any) {
		log_cache = &logCache{
			lru: ttlcache.NewCache(),
		}
		log_cache.lru.SetCacheSizeLimit(100)

	} else {
		log_cache, _ = log_cache_any.(*logCache)
		if log_cache == nil {
			// Cant really happen
			return false
		}
	}

	now := utils.GetTime().Now().Unix()

	// Was this message emitted recently?
	log_cache_entry_any, err := log_cache.lru.Get(arg.Message)
	if err == nil {
		log_cache_entry, ok := log_cache_entry_any.(*logCacheEntry)

		// Message is identical to last and within the dedup time.
		if ok && arg.DedupTime > 0 &&
			log_cache_entry.last_time+arg.DedupTime > now {
			return true
		}
	}

	// Store the message in the cache for next time
	log_cache.lru.Set(arg.Message, &logCacheEntry{
		last_time: now,
	})

	vql_subsystem.CacheSet(scope, LOG_TAG, log_cache)

	// Go ahead and format the message now

	level := strings.ToUpper(arg.Level)
	switch level {
	case logging.DEFAULT, logging.ERROR, logging.INFO,
		logging.WARNING, logging.DEBUG, logging.ALERT:

	default:
		level = logging.DEFAULT
	}

	message := fmt.Sprintf("%s:%s", level, arg.Message)
	if !utils.IsNil(arg.Args) {
		slice := reflect.ValueOf(arg.Args)
		var format_args []interface{}

		// Not a slice - we just format the object as is
		if slice.Type().Kind() != reflect.Slice {
			format_args = append(format_args, arg.Args)
		} else {
			for i := 0; i < slice.Len(); i++ {
				value := slice.Index(i).Interface()
				format_args = append(format_args, value)
			}
		}
		message = fmt.Sprintf(message, format_args...)
	}
	scope.Log("%v", message)
	return true
}

func (self LogFunction) Info(scope vfilter.Scope, type_map *vfilter.TypeMap) *vfilter.FunctionInfo {
	return &vfilter.FunctionInfo{
		Name:    "log",
		Doc:     "Log the message.",
		ArgType: type_map.AddType(scope, &LogFunctionArgs{}),
		Version: 2,
	}
}

func init() {
	vql_subsystem.RegisterFunction(&LogFunction{})
}
