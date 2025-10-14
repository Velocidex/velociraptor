package functions

import (
	"context"
	"errors"
	"math"
	"strings"
	"time"

	"www.velocidex.com/golang/vfilter/protocols"
	"www.velocidex.com/golang/vfilter/types"

	"github.com/Velocidex/ordereddict"
	"github.com/araddon/dateparse"
	"www.velocidex.com/golang/velociraptor/constants"
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
	invalidTimeError     = errors.New("Invalid time")
	exported_time_fields = []string{
		"Day", "Hour", "ISOWeek", "IsDST", "IsZero", "Minute",
		"Month", "Nanosecond", "Second", "String", "UTC",
		"Unix", "UnixMicro", "UnixMilli", "UnixNano",
		"Weekday", "Year", "YearDay", "Zone"}
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

func getTimezone(
	ctx context.Context, scope types.Scope) (*time.Location, string) {
	// Is there a Parse TZ first?
	tz, pres := scope.Resolve("PARSE_TZ")
	if pres {
		tz = vql_subsystem.Materialize(ctx, scope, tz)
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
	tz, pres = scope.Resolve(constants.TZ)
	if pres {
		tz = vql_subsystem.Materialize(ctx, scope, tz)
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

func GetTimeCache(
	ctx context.Context,
	scope types.Scope) *TimestampCache {
	cache_ctx, ok := vql_subsystem.CacheGet(scope, timecache_key).(*TimestampCache)
	if !ok {
		loc, tz := getTimezone(ctx, scope)
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
	Format      string      `vfilter:"optional,field=format,doc=A format specifier as per the Golang time.Parse"`
}

type _Timestamp struct{}

func (self _Timestamp) Info(scope vfilter.Scope, type_map *vfilter.TypeMap) *vfilter.FunctionInfo {
	return &vfilter.FunctionInfo{
		Name:    "timestamp",
		Doc:     "Convert from different types to a time.Time.",
		ArgType: type_map.AddType(scope, _TimestampArg{}),
		Version: 2,
	}
}

func (self _Timestamp) Call(ctx context.Context, scope vfilter.Scope,
	args *ordereddict.Dict) vfilter.Any {

	defer vql_subsystem.RegisterMonitor(ctx, "timestamp", args)()

	arg := &_TimestampArg{}
	err := arg_parser.ExtractArgsWithContext(ctx, scope, args, arg)
	if err != nil {
		scope.Log("timestamp: %s", err.Error())
		return vfilter.Null{}
	}

	if arg.CocoaTime > 0 {
		return time.Unix((arg.CocoaTime + 978307200), 0).UTC()
	}

	if arg.MacTime > 0 {
		return time.Unix((arg.MacTime - 2082844800), 0).UTC()
	}

	if arg.WinFileTime > 0 {
		return utils.WinFileTime(arg.WinFileTime)
	}

	if arg.String != "" {
		arg.Epoch = arg.String
	}

	// Use traditional but limited golang time.Parse
	if arg.Format != "" {
		str, ok := arg.Epoch.(string)
		if ok {
			result, err := ParseTimeFromStringWithFormat(
				ctx, scope, arg.Format, str)
			if err != nil {
				return vfilter.Null{}
			}
			return result.UTC()
		}
	}

	result, err := TimeFromAny(ctx, scope, arg.Epoch)
	if err != nil {
		return vfilter.Null{}
	}

	return result.UTC()
}

func TimeFromAny(ctx context.Context,
	scope vfilter.Scope, timestamp vfilter.Any) (time.Time, error) {
	sec := int64(0)
	dec := int64(0)
	switch t := timestamp.(type) {
	case vfilter.LazyExpr:
		return TimeFromAny(ctx, scope, t.ReduceWithScope(ctx, scope))

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
		// It might really be an int encoded as a string.
		int_time, ok := utils.ToInt64(t)
		if ok {
			return TimeFromAny(ctx, scope, int_time)
		}

		return ParseTimeFromString(ctx, scope, t)

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

		return utils.ParseTimeFromInt64(sec), nil
	}

	// Empty times are allowed, they will just be set to the earliest
	// time we have (Note this is not the epoch!).
	if sec == 0 && dec == 0 {
		return time.Time{}, nil
	}

	return time.Unix(int64(sec), int64(dec)), nil
}

func ParseTimeFromString(
	ctx context.Context,
	scope vfilter.Scope, timestamp string) (
	time_value time.Time, err error) {

	cache := GetTimeCache(ctx, scope)
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

func ParseTimeFromStringWithFormat(
	ctx context.Context,
	scope vfilter.Scope, format, timestamp string) (
	time_value time.Time, err error) {

	cache := GetTimeCache(ctx, scope)
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

// Time aware operators. Automatically coerce strings as time objects
// when compared to time objects.
type _TimeLtString struct{}

func (self _TimeLtString) getTimes(
	ctx context.Context,
	scope vfilter.Scope,
	a vfilter.Any, b vfilter.Any) (time.Time, time.Time, bool) {
	var b_time time.Time
	var b_is_time, ok bool
	var err error

	cache := GetTimeCache(ctx, scope)

	a_time, a_is_time := utils.IsTime(a)
	b_str, b_is_str := b.(string)

	// maybe the sense is reversed
	if !a_is_time || !b_is_str {
		b_time, b_is_time = utils.IsTime(b)
		a_str, a_is_str := a.(string)
		if !b_is_time || !a_is_str {
			// Should not happen since Applicable should catch it.
			return a_time, b_time, false
		}

		a_time, ok = cache.Get(scope, a_str)
		if !ok {
			a_time, err = ParseTimeFromString(ctx, scope, a_str)
			cache.Set(scope, a_str, a_time)
			if err != nil {
				return a_time, b_time, false
			}
		}
	} else {
		b_time, ok = cache.Get(scope, b_str)
		if !ok {
			// If we can not parse the string properly return false.
			b_time, err = ParseTimeFromString(ctx, scope, b_str)
			cache.Set(scope, b_str, b_time)
			if err != nil {
				return a_time, b_time, false
			}
		}
	}

	return a_time, b_time, true
}

func (self _TimeLtString) Lt(scope vfilter.Scope, a vfilter.Any, b vfilter.Any) bool {
	a_time, b_time, ok := self.getTimes(context.Background(), scope, a, b)
	if !ok {
		return false
	}
	return a_time.Before(b_time)
}

func (self _TimeLtString) Applicable(a vfilter.Any, b vfilter.Any) bool {
	_, a_time := utils.IsTime(a)
	_, b_str := b.(string)

	if a_time && b_str {
		return true
	}

	_, b_time := utils.IsTime(b)
	_, a_str := a.(string)
	return b_time && a_str
}

type _TimeGtString struct{}

func (self _TimeGtString) Applicable(a vfilter.Any, b vfilter.Any) bool {
	return _TimeLtString{}.Applicable(a, b)
}

func (self _TimeGtString) Gt(scope vfilter.Scope, a vfilter.Any, b vfilter.Any) bool {
	a_time, b_time, ok := _TimeLtString{}.getTimes(context.Background(),
		scope, a, b)
	if !ok {
		return false
	}
	return a_time.After(b_time)
}

type _TimeAssociative struct{}

// Filter some method calls to be more useful.
func (self _TimeAssociative) Associative(scope vfilter.Scope, a vfilter.Any, b vfilter.Any) (vfilter.Any, bool) {
	a_time, ok := utils.IsTime(a)
	if !ok {
		return &vfilter.Null{}, false
	}

	method, ok := b.(string)
	if !ok {
		return &vfilter.Null{}, false
	}

	if method == "String" {
		return a_time.UTC().Format(time.RFC3339), true
	}

	return protocols.DefaultAssociative{}.Associative(scope, a, method)
}

func (self _TimeAssociative) Applicable(a vfilter.Any, b vfilter.Any) bool {
	_, a_ok := utils.IsTime(a)
	if !a_ok {
		return false
	}

	_, ok := b.(string)
	return ok
}

func (self _TimeAssociative) GetMembers(scope vfilter.Scope, a vfilter.Any) []string {
	return exported_time_fields
}

type _TimestampFormatArg struct {
	Time   vfilter.Any `vfilter:"required,field=time,doc=Time to format"`
	Format string      `vfilter:"optional,field=format,doc=A format specifier as per the Golang time.Format. Additionally any constants specified in https://pkg.go.dev/time#pkg-constants can be used."`
}

type _TimestampFormat struct{}

func (self _TimestampFormat) Info(scope vfilter.Scope, type_map *vfilter.TypeMap) *vfilter.FunctionInfo {
	return &vfilter.FunctionInfo{
		Name:    "timestamp_format",
		Doc:     "Format a timestamp into a string.",
		ArgType: type_map.AddType(scope, _TimestampFormatArg{}),
		Version: 1,
	}
}

var (
	Layouts = map[string]string{
		"Layout":      time.Layout,
		"ANSIC":       time.ANSIC,
		"UnixDate":    time.UnixDate,
		"RubyDate":    time.RubyDate,
		"RFC822":      time.RFC822,
		"RFC822Z":     time.RFC822Z,
		"RFC850":      time.RFC850,
		"RFC1123":     time.RFC1123,
		"RFC1123Z":    time.RFC1123Z,
		"RFC3339":     time.RFC3339,
		"RFC3339Nano": time.RFC3339Nano,
		"Kitchen":     time.Kitchen,
		"Stamp":       time.Stamp,
		"StampMilli":  time.StampMilli,
		"StampMicro":  time.StampMicro,
		"StampNano":   time.StampNano,
		"DateTime":    time.DateTime,
		"DateOnly":    time.DateOnly,
		"TimeOnly":    time.TimeOnly,
	}
)

func (self _TimestampFormat) Call(ctx context.Context, scope vfilter.Scope,
	args *ordereddict.Dict) vfilter.Any {

	defer vql_subsystem.RegisterMonitor(ctx, "timestamp_format", args)()

	arg := &_TimestampFormatArg{}
	err := arg_parser.ExtractArgsWithContext(ctx, scope, args, arg)
	if err != nil {
		scope.Log("timestamp_format: %s", err.Error())
		return vfilter.Null{}
	}

	timestamp, err := TimeFromAny(ctx, scope, arg.Time)
	if err != nil {
		return vfilter.Null{}
	}

	format, pres := Layouts[arg.Format]
	if !pres {
		format = arg.Format
	}

	if format == "" {
		format = time.RFC3339
	}

	loc := GetTimeCache(ctx, scope).loc

	// Format the time in the required timezone.
	return timestamp.In(loc).Format(format)
}

func init() {
	vql_subsystem.RegisterFunction(&_Timestamp{})
	vql_subsystem.RegisterFunction(&_TimestampFormat{})
	vql_subsystem.RegisterProtocol(&_TimeLtString{})
	vql_subsystem.RegisterProtocol(&_TimeGtString{})
	vql_subsystem.RegisterProtocol(&_TimeAssociative{})
}
