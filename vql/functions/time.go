package functions

import (
	"context"
	"errors"
	"math"
	"strings"
	"time"

	"www.velocidex.com/golang/vfilter/types"

	"github.com/Velocidex/ordereddict"
	"github.com/araddon/dateparse"
	"www.velocidex.com/golang/velociraptor/third_party/cache"
	"www.velocidex.com/golang/velociraptor/utils"
	vql_subsystem "www.velocidex.com/golang/velociraptor/vql"
	"www.velocidex.com/golang/vfilter"
	"www.velocidex.com/golang/vfilter/arg_parser"

	// Force timezone database to be compiled in.
	_ "time/tzdata"
)

const (
	timecache_key = "$timestamp_cache"
)

var (
	invalidTimeError = errors.New("Invalid time")
)

type cachedTime struct {
	time.Time
}

func (self cachedTime) Size() int {
	return 1
}

type TimestampCache struct {
	lru   *cache.LRUCache
	tz    string
	loc   *time.Location
	debug bool
}

func getDebug(scope types.Scope) bool {
	_, pres := scope.Resolve("DEBUG")
	return pres
}

func getTimezone(scope types.Scope) (*time.Location, string) {
	// Is there a Parse TZ first?
	tz, pres := scope.Resolve("PARSE_TZ")
	if pres {
		tz_str, ok := tz.(string)
		if ok {
			// Get the local time whatever it might be
			if strings.ToLower(tz_str) == "local" {
				return time.Local, time.Local.String()
			}

			loc, err := time.LoadLocation(tz_str)
			if err != nil {
				// Unable to load location - maybe invalid.
				scope.Log("Unable to load timezone from PARSE_TZ %v: %v, using UTC",
					tz_str, err)
				return time.UTC, ""
			}
			return loc, tz_str
		}
	}

	// Otherwise the user may have specified a global timezone (which
	// also affects output).
	tz, pres = scope.Resolve("TZ")
	if pres {
		tz_str, ok := tz.(string)
		if ok {
			loc, err := time.LoadLocation(tz_str)
			if err != nil {
				// Unable to load location - maybe invalid.
				scope.Log("Unable to load timezone from TZ %v: %v, using UTC",
					tz_str, err)
				return time.UTC, ""
			}
			return loc, tz_str
		}
	}

	return time.UTC, ""
}

func (self *TimestampCache) Get(scope types.Scope, timestamp string) (
	time.Time, bool) {

	lru_key := self.tz + timestamp
	time_value_any, pres := self.lru.Get(lru_key)
	if pres {
		return time_value_any.(cachedTime).Time, true
	}
	return time.Time{}, false
}

func (self *TimestampCache) Set(scope types.Scope,
	timestamp string, value time.Time) {
	lru_key := self.tz + timestamp
	self.lru.Set(lru_key, cachedTime{value})
}

func GetTimeCache(scope types.Scope) *TimestampCache {
	cache_ctx, ok := vql_subsystem.CacheGet(scope, timecache_key).(*TimestampCache)
	if !ok {
		loc, tz := getTimezone(scope)
		cache_ctx = &TimestampCache{
			lru:   cache.NewLRUCache(200),
			loc:   loc,
			tz:    tz,
			debug: getDebug(scope),
		}
		vql_subsystem.CacheSet(scope, timecache_key, cache_ctx)
	}
	return cache_ctx
}

type _TimestampArg struct {
	Epoch       vfilter.Any `vfilter:"optional,field=epoch"`
	CocoaTime   int64       `vfilter:"optional,field=cocoatime"`
	MacTime     int64       `vfilter:"optional,field=mactime,doc=HFS+"`
	WinFileTime int64       `vfilter:"optional,field=winfiletime"`
	String      string      `vfilter:"optional,field=string,doc=Guess a timestamp from a string"`
	Timezone    string      `vfilter:"optional,field=timezone,doc=A default timezone (UTC)"`
	Format      string      `vfilter:"optional,field=format,doc=A format specifier as per the Golang time.Parse"`
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
	err := arg_parser.ExtractArgsWithContext(ctx, scope, args, arg)
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

	// Use traditional but limited golang time.Parse
	if arg.Format != "" {
		str, ok := arg.Epoch.(string)
		if ok {
			result, err := ParseTimeFromStringWithFormat(scope, arg.Format, str)
			if err != nil {
				return vfilter.Null{}
			}
			return result
		}
	}

	result, err := TimeFromAny(scope, arg.Epoch)
	if err != nil {
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
		// (This is not the unix epoch!)
		if t == "" {
			return time.Time{}, nil
		}
		return ParseTimeFromString(scope, t)

	case time.Time:
		return t, nil

	case *time.Time:
		return *t, nil

	case nil, types.Null, *types.Null:
		return time.Time{}, invalidTimeError

	default:
		var ok bool

		// Can we convert it to an int?
		sec, ok = utils.ToInt64(timestamp)
		if !ok {
			return time.Time{}, invalidTimeError
		}

		// Maybe it is in ns
		if sec > 20000000000000000 { // 11 October 2603 in microsec
			dec = sec
			sec = 0

		} else if sec > 20000000000000 { // 11 October 2603 in milliseconds
			dec = sec * 1000
			sec = 0

		} else if sec > 20000000000 { // 11 October 2603 in seconds
			dec = sec * 1000000
			sec = 0
		}
	}

	// Empty times are allowed, they will just be set to the earliest
	// time we have (Note this is not the epoch!).
	if sec == 0 && dec == 0 {
		return time.Time{}, invalidTimeError
	}

	return time.Unix(int64(sec), int64(dec)), nil
}

func ParseTimeFromString(scope vfilter.Scope, timestamp string) (
	time_value time.Time, err error) {

	cache := GetTimeCache(scope)
	cached_time_value, pres := cache.Get(scope, timestamp)
	if pres {
		return cached_time_value, nil
	}

	if cache.loc != nil {
		time_value, err = dateparse.ParseIn(timestamp, cache.loc)
	} else {
		time_value, err = dateparse.ParseAny(timestamp)
	}

	if err != nil && cache.debug {
		scope.Log("Parsing timestamp %v: %v", timestamp, err)
	}

	// Update the LRU
	cache.Set(scope, timestamp, time_value)

	return time_value, err
}

func ParseTimeFromStringWithFormat(scope vfilter.Scope, format, timestamp string) (
	time_value time.Time, err error) {

	cache := GetTimeCache(scope)
	time_value, pres := cache.Get(scope, timestamp)
	if pres {
		return time_value, nil
	}

	if cache.loc != nil {
		time_value, err = time.ParseInLocation(format, timestamp, cache.loc)
	} else {
		time_value, err = time.Parse(format, timestamp)
	}

	if err != nil && cache.debug {
		scope.Log("Parsing timestamp %v: %v", timestamp, err)
	}

	// Update the LRU on return.
	cache.Set(scope, timestamp, time_value)

	return time_value, err
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
	var err error

	cache := GetTimeCache(scope)
	b_time, pres := cache.Get(scope, b_str)
	if !pres {
		// If we can not parse the string properly return false.
		b_time, err = ParseTimeFromString(scope, b_str)
		cache.Set(scope, b_str, b_time)
		if err != nil {
			return false
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
	var err error

	cache := GetTimeCache(scope)
	b_time, pres := cache.Get(scope, b_str)
	if !pres {
		// If we can not parse the string properly return false.
		b_time, err = ParseTimeFromString(scope, b_str)
		cache.Set(scope, b_str, b_time)
		if err != nil {
			return false
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
