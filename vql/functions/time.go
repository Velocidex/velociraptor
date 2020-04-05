package functions

import (
	"context"
	"math"
	"time"

	"github.com/Velocidex/ordereddict"
	"github.com/kierdavis/dateparser"
	"www.velocidex.com/golang/velociraptor/third_party/cache"
	"www.velocidex.com/golang/velociraptor/utils"
	vql_subsystem "www.velocidex.com/golang/velociraptor/vql"
	"www.velocidex.com/golang/vfilter"
)

var (
	lru *cache.LRUCache = cache.NewLRUCache(100)
)

type cachedTime struct {
	utils.Time
}

func (self cachedTime) Size() int {
	return 1
}

type _TimestampArg struct {
	Epoch       vfilter.Any `vfilter:"optional,field=epoch"`
	WinFileTime int64       `vfilter:"optional,field=winfiletime"`
	String      string      `vfilter:"optional,field=string,doc=Guess a timestamp from a string"`
	UsStyle     bool        `vfilter:"optional,field=us_style,doc=US Style Month/Day/Year"`
}
type _Timestamp struct{}

func (self _Timestamp) Info(scope *vfilter.Scope, type_map *vfilter.TypeMap) *vfilter.FunctionInfo {
	return &vfilter.FunctionInfo{
		Name:    "timestamp",
		Doc:     "Convert from different types to a time.Time.",
		ArgType: type_map.AddType(scope, _TimestampArg{}),
	}
}

func (self _Timestamp) Call(ctx context.Context, scope *vfilter.Scope,
	args *ordereddict.Dict) vfilter.Any {
	arg := &_TimestampArg{}
	err := vfilter.ExtractArgs(scope, args, arg)
	if err != nil {
		scope.Log("timestamp: %s", err.Error())
		return vfilter.Null{}
	}

	if arg.WinFileTime > 0 {
		return time.Unix((arg.WinFileTime/10000000)-11644473600, 0)
	}

	if arg.String != "" {
		time_value_any, pres := lru.Get(arg.String)
		if pres {
			return time_value_any.(cachedTime).Time
		}

		parser := dateparser.Parser{Fuzzy: true,
			DayFirst: true,
			IgnoreTZ: true}
		time_value, err := parser.Parse(arg.String)
		if err == nil {
			lru.Set(arg.String, cachedTime{utils.Time{time_value}})
			return time_value
		}
		scope.Log("timestamp: %v", err)
		return vfilter.Null{}
	}

	sec := int64(0)
	dec := int64(0)
	switch t := arg.Epoch.(type) {
	case float64:
		sec_f, dec_f := math.Modf(t)
		sec = int64(sec_f)
		dec = int64(dec_f * 1e9)
	default:
		sec, _ = utils.ToInt64(arg.Epoch)
	}

	// Propagate a null to output.
	if sec == 0 && dec == 0 {
		return vfilter.Null{}
	}

	return utils.Unix(int64(sec), int64(dec))
}

// Time aware operators.
type _TimeLt struct{}

func (self _TimeLt) Lt(scope *vfilter.Scope, a vfilter.Any, b vfilter.Any) bool {
	a_time, _ := utils.IsTime(a)
	b_time, _ := utils.IsTime(b)

	return a_time.Before(b_time)
}

func (self _TimeLt) Applicable(a vfilter.Any, b vfilter.Any) bool {
	_, a_ok := utils.IsTime(a)
	_, b_ok := utils.IsTime(b)

	return a_ok && b_ok
}

type _TimeLtInt struct{}

func (self _TimeLtInt) Lt(scope *vfilter.Scope, a vfilter.Any, b vfilter.Any) bool {
	a_time, _ := utils.IsTime(a)
	var b_time utils.Time

	switch t := b.(type) {
	case float64:
		sec_f, dec_f := math.Modf(t)
		dec_f *= 1e9
		b_time = utils.Unix(int64(sec_f), int64(dec_f))
	default:
		sec, _ := utils.ToInt64(b)
		b_time = utils.Unix(sec, 0)
	}

	return a_time.Before(b_time)
}

func (self _TimeLtInt) Applicable(a vfilter.Any, b vfilter.Any) bool {
	_, a_ok := utils.IsTime(a)
	if !a_ok {
		return false
	}

	_, ok := utils.ToInt64(b)
	return ok
}

type _TimeLtString struct{}

func (self _TimeLtString) Lt(scope *vfilter.Scope, a vfilter.Any, b vfilter.Any) bool {
	a_time, _ := utils.IsTime(a)
	b_str, _ := b.(string)
	var b_time utils.Time

	time_value_any, pres := lru.Get(b_str)
	if pres {
		b_time = time_value_any.(cachedTime).Time

	} else {
		parser := dateparser.Parser{Fuzzy: true,
			DayFirst: true,
			IgnoreTZ: true}
		b_time_time, err := parser.Parse(b_str)
		if err == nil {
			b_time = utils.Time{b_time_time}
			lru.Set(b_str, cachedTime{b_time})
		}
	}

	return a_time.Before(b_time)
}

func (self _TimeLtString) Applicable(a vfilter.Any, b vfilter.Any) bool {
	_, a_ok := utils.IsTime(a)
	_, b_ok := b.(string)

	return a_ok && b_ok
}

type _TimeEq struct{}

func (self _TimeEq) Eq(scope *vfilter.Scope, a vfilter.Any, b vfilter.Any) bool {
	a_time, _ := utils.IsTime(a)
	b_time, _ := utils.IsTime(b)

	return a_time == b_time
}

func (self _TimeEq) Applicable(a vfilter.Any, b vfilter.Any) bool {
	_, a_ok := utils.IsTime(a)
	_, b_ok := utils.IsTime(b)

	return a_ok && b_ok
}

type _TimeEqInt struct{}

func (self _TimeEqInt) Eq(scope *vfilter.Scope, a vfilter.Any, b vfilter.Any) bool {
	a_time, _ := utils.IsTime(a)
	var b_time time.Time

	switch t := b.(type) {
	case float64:
		sec_f, dec_f := math.Modf(t)
		dec_f *= 1e9
		b_time = time.Unix(int64(sec_f), int64(dec_f))
	default:
		sec, _ := utils.ToInt64(b)
		b_time = time.Unix(sec, 0)
	}

	return a_time.UnixNano() == b_time.UnixNano()
}

func (self _TimeEqInt) Applicable(a vfilter.Any, b vfilter.Any) bool {
	_, a_ok := utils.IsTime(a)
	if !a_ok {
		return false
	}

	_, ok := utils.ToInt64(b)
	return ok
}

func init() {
	vql_subsystem.RegisterFunction(&_Timestamp{})
	vql_subsystem.RegisterProtocol(&_TimeLt{})
	vql_subsystem.RegisterProtocol(&_TimeLtInt{})
	vql_subsystem.RegisterProtocol(&_TimeLtString{})
	vql_subsystem.RegisterProtocol(&_TimeEq{})
	vql_subsystem.RegisterProtocol(&_TimeEqInt{})
}
