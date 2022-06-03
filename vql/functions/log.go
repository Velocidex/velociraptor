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
package functions

import (
	"context"
	"fmt"
	"reflect"
	"strings"
	"time"

	"github.com/Velocidex/ordereddict"
	"www.velocidex.com/golang/velociraptor/logging"
	"www.velocidex.com/golang/velociraptor/utils"
	vql_subsystem "www.velocidex.com/golang/velociraptor/vql"
	"www.velocidex.com/golang/vfilter"
	"www.velocidex.com/golang/vfilter/arg_parser"
)

const (
	LOG_TAG = "last_log"
)

type logCache struct {
	message string
	time    int64
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

	now := time.Now().Unix()
	level := strings.ToUpper(arg.Level)
	switch level {
	case logging.DEFAULT, logging.ERROR, logging.INFO,
		logging.WARNING, logging.DEBUG:

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

	last_log_any := vql_subsystem.CacheGet(scope, LOG_TAG)

	// No previous message was set - log it and save it.
	if utils.IsNil(last_log_any) {
		last_log := &logCache{
			message: message,
			time:    now,
		}
		scope.Log("%v", message)
		vql_subsystem.CacheSet(scope, LOG_TAG, last_log)
		return true
	}

	last_log, ok := last_log_any.(*logCache)
	// Message is identical to last and within the dedup time.
	if ok && last_log.message == message &&
		arg.DedupTime > 0 && // User can set dedup time negative to disable.
		now < last_log.time+arg.DedupTime {
		return true
	}

	// Log it and store for next time.
	scope.Log("%v", message)
	vql_subsystem.CacheSet(scope, LOG_TAG, &logCache{
		message: message,
		time:    now,
	})

	return true
}

func (self LogFunction) Info(scope vfilter.Scope, type_map *vfilter.TypeMap) *vfilter.FunctionInfo {
	return &vfilter.FunctionInfo{
		Name:    "log",
		Doc:     "Log the message.",
		ArgType: type_map.AddType(scope, &LogFunctionArgs{}),
	}
}

func init() {
	vql_subsystem.RegisterFunction(&LogFunction{})
}
