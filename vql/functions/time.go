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
	time.Time
}

func (self cachedTime) Size() int {
	return 1
}

type _TimestampArg struct {
	Epoch       vfilter.Any `vfilter:"optional,field=epoch"`
	CocoaTime   int64       `vfilter:"optional,field=cocoatime"`
	MacTime     int64       `vfilter:"optional,field=mactime,doc=HFS+"`
	WinFileTime int64       `vfilter:"optional,field=winfiletime"`
	String      string      `vfilter:"optional,field=string,doc=Guess a timestamp from a string"`
	UsStyle     bool        `vfilter:"optional,field=us_style,doc=US Style Month/Day/Year"`
}

type _Timestamp struct{}

func (self _Timestamp) Info(scope vfilter.Scope, type_map *vfilter.TypeMap) *vfilter.FunctionInfo {
	return &vfilter.FunctionInfo{
		Name:    "timestamp",
		Doc:     "Convert from different types to a time.Time.",
		ArgType: type_map.AddType(scope, _TimestampArg{}),
	}
}

func (self _Timestamp) Call(ctx context.Context, scope vfilter.Scope,
	args *ordereddict.Dict) vfilter.Any {
	arg := &_TimestampArg{}
	err := vfilter.ExtractArgs(scope, args, arg)
	if err != nil {
		scope.Log("timestamp: %s", err.Error())
		return vfilter.Null{}
	}

	if arg.CocoaTime > 0 {
		return time.Unix((arg.CocoaTime + 978307200), 0)
	}

	if arg.MacTime > 0 {
		return time.Unix((arg.MacTime - 2082844800), 0)
	}

	if arg.WinFileTime > 0 {
		return time.Unix((arg.WinFileTime/10000000)-11644473600, 0)
	}

	if arg.String != "" {
		arg.Epoch = arg.String
	}

	result, err := TimeFromAny(scope, arg.Epoch)
	if err != nil || result.Unix() == 0 {
		return vfilter.Null{}
	}

	return result
}

func TimeFromAny(scope vfilter.Scope, timestamp vfilter.Any) (time.Time, error) {
	sec := int64(0)
	dec := int64(0)
	switch t := timestamp.(type) {
	case float64:
		sec_f, dec_f := math.Modf(t)
		sec = int64(sec_f)
		dec = int64(dec_f * 1e9)

	case string:
		// If there is no input return an empty timestamp
		// (unix epoch)
		if t == "" {
			return time.Time{}, nil
		}
		return parse_time_from_string(scope, t)

	case time.Time:
		return t, nil

	case *time.Time:
		return *t, nil

	case *utils.TimeVal:
		return t.Time(), nil

	case utils.TimeVal:
		return t.Time(), nil

	default:
		sec, _ = utils.ToInt64(timestamp)
	}

	return time.Unix(int64(sec), int64(dec)), nil
}

func parse_time_from_string(scope vfilter.Scope, timestamp string) (
	time.Time, error) {
	time_value_any, pres := lru.Get(timestamp)
	if pres {
		return time_value_any.(cachedTime).Time, nil
	}

	parser := dateparser.Parser{Fuzzy: true, DayFirst: true, IgnoreTZ: true}
	time_value, err := parser.Parse(timestamp)
	if err != nil {
		return time_value, err
	}
	lru.Set(timestamp, cachedTime{time_value})
	return time_value, nil
}

// Time aware operators.
type _TimeLt struct{}

// a < b
func (self _TimeLt) Lt(scope vfilter.Scope, a vfilter.Any, b vfilter.Any) bool {
	a_time, _ := utils.IsTime(a)
	b_time, _ := utils.IsTime(b)

	return a_time.Before(b_time)
}

func (self _TimeLt) Applicable(a vfilter.Any, b vfilter.Any) bool {
	_, a_ok := utils.IsTime(a)
	_, b_ok := utils.IsTime(b)

	return a_ok && b_ok
}

type _TimeGt struct{}

// a > b
func (self _TimeGt) Gt(scope vfilter.Scope, a vfilter.Any, b vfilter.Any) bool {
	a_time, _ := utils.IsTime(a)
	b_time, _ := utils.IsTime(b)

	return a_time.After(b_time)
}

func (self _TimeGt) Applicable(a vfilter.Any, b vfilter.Any) bool {
	_, a_ok := utils.IsTime(a)
	_, b_ok := utils.IsTime(b)

	return a_ok && b_ok
}

type _TimeLtInt struct{}

func (self _TimeLtInt) Lt(scope vfilter.Scope, a vfilter.Any, b vfilter.Any) bool {
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

type _TimeGtInt struct{}

func (self _TimeGtInt) Gt(scope vfilter.Scope, a vfilter.Any, b vfilter.Any) bool {
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

	return a_time.After(b_time)
}

func (self _TimeGtInt) Applicable(a vfilter.Any, b vfilter.Any) bool {
	_, a_ok := utils.IsTime(a)
	if !a_ok {
		return false
	}

	_, ok := utils.ToInt64(b)
	return ok
}

type _TimeLtString struct{}

func (self _TimeLtString) Lt(scope vfilter.Scope, a vfilter.Any, b vfilter.Any) bool {
	a_time, _ := utils.IsTime(a)
	b_str, _ := b.(string)
	var b_time time.Time

	time_value_any, pres := lru.Get(b_str)
	if pres {
		b_time = time_value_any.(cachedTime).Time

	} else {
		parser := dateparser.Parser{Fuzzy: true,
			DayFirst: true,
			IgnoreTZ: true}
		b_time_time, err := parser.Parse(b_str)
		if err == nil {
			b_time = b_time_time
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

type _TimeGtString struct{}

func (self _TimeGtString) Gt(scope vfilter.Scope, a vfilter.Any, b vfilter.Any) bool {
	a_time, _ := utils.IsTime(a)
	b_str, _ := b.(string)
	var b_time time.Time

	time_value_any, pres := lru.Get(b_str)
	if pres {
		b_time = time_value_any.(cachedTime).Time

	} else {
		parser := dateparser.Parser{Fuzzy: true,
			DayFirst: true,
			IgnoreTZ: true}
		b_time_time, err := parser.Parse(b_str)
		if err == nil {
			b_time = b_time_time
			lru.Set(b_str, cachedTime{b_time})
		}
	}

	return a_time.After(b_time)
}

func (self _TimeGtString) Applicable(a vfilter.Any, b vfilter.Any) bool {
	_, a_ok := utils.IsTime(a)
	_, b_ok := b.(string)

	return a_ok && b_ok
}

type _TimeEq struct{}

func (self _TimeEq) Eq(scope vfilter.Scope, a vfilter.Any, b vfilter.Any) bool {
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

func (self _TimeEqInt) Eq(scope vfilter.Scope, a vfilter.Any, b vfilter.Any) bool {
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
	vql_subsystem.RegisterProtocol(&_TimeGt{})
	vql_subsystem.RegisterProtocol(&_TimeLtInt{})
	vql_subsystem.RegisterProtocol(&_TimeGtInt{})
	vql_subsystem.RegisterProtocol(&_TimeLtString{})
	vql_subsystem.RegisterProtocol(&_TimeGtString{})
	vql_subsystem.RegisterProtocol(&_TimeEq{})
	vql_subsystem.RegisterProtocol(&_TimeEqInt{})
}
