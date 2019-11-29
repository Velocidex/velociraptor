package functions

import (
	"context"
	"time"

	"github.com/Velocidex/ordereddict"
	"github.com/olebedev/when"
	"github.com/olebedev/when/rules"
	"github.com/olebedev/when/rules/common"
	"github.com/olebedev/when/rules/en"
	vql_subsystem "www.velocidex.com/golang/velociraptor/vql"
	"www.velocidex.com/golang/vfilter"
)

type _TimestampArg struct {
	Epoch       int64  `vfilter:"optional,field=epoch"`
	WinFileTime int64  `vfilter:"optional,field=winfiletime"`
	String      string `vfilter:"optional,field=string,doc=Guess a timestamp from a string"`
	UsStyle     bool   `vfilter:"optional,field=us_style,doc=US Style Month/Day/Year"`
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

	if arg.Epoch > 0 {
		return time.Unix(arg.Epoch, 0)
	}

	if arg.WinFileTime > 0 {
		return time.Unix((arg.WinFileTime/10000000)-11644473600, 0)
	}

	if arg.String != "" {
		w := when.New(nil)
		w.Add(SlashMDY(rules.Override, arg.UsStyle))
		w.Add(en.All...)
		w.Add(common.All...)

		r, err := w.Parse(arg.String, time.Now())
		if err == nil && r != nil {
			return r.Time
		}
		scope.Log("timestamp: %v", err)
	}

	return vfilter.Null{}
}

// Time aware operators.
type _TimeLt struct{}

func (self _TimeLt) Lt(scope *vfilter.Scope, a vfilter.Any, b vfilter.Any) bool {
	a_time, _ := a.(time.Time)
	b_time, _ := b.(time.Time)

	return a_time.Before(b_time)
}

func (self _TimeLt) Applicable(a vfilter.Any, b vfilter.Any) bool {
	_, a_ok := a.(time.Time)
	_, b_ok := b.(time.Time)

	return a_ok && b_ok
}

type _TimeEq struct{}

func (self _TimeEq) Eq(scope *vfilter.Scope, a vfilter.Any, b vfilter.Any) bool {
	a_time, _ := a.(time.Time)
	b_time, _ := b.(time.Time)

	return a_time == b_time
}

func (self _TimeEq) Applicable(a vfilter.Any, b vfilter.Any) bool {
	_, a_ok := a.(time.Time)
	_, b_ok := b.(time.Time)

	return a_ok && b_ok
}

func init() {
	vql_subsystem.RegisterFunction(&_Timestamp{})
	vql_subsystem.RegisterProtocol(&_TimeLt{})
	vql_subsystem.RegisterProtocol(&_TimeEq{})
}
