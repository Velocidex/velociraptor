package protocols

import (
	"context"
	"encoding/json"
	"reflect"

	"github.com/Velocidex/ordereddict"
	vql_subsystem "www.velocidex.com/golang/velociraptor/vql"
	"www.velocidex.com/golang/vfilter"
	"www.velocidex.com/golang/vfilter/types"
)

type _BoolDict struct{}

func (self _BoolDict) Applicable(a vfilter.Any) bool {
	switch a.(type) {
	case ordereddict.Dict, *ordereddict.Dict:
		return true
	}

	rt := reflect.TypeOf(a)
	if rt == nil {
		return false
	}
	return rt.Kind() == reflect.Slice || rt.Kind() == reflect.Map
}

func (self _BoolDict) Bool(ctx context.Context, scope vfilter.Scope, a vfilter.Any) bool {
	switch t := a.(type) {
	case ordereddict.Dict:
		return t.Len() > 0
	case *ordereddict.Dict:
		return t.Len() > 0
	}

	rt := reflect.TypeOf(a)
	if rt == nil {
		return false
	}
	if rt.Kind() == reflect.Slice || rt.Kind() == reflect.Map {
		a_slice := reflect.ValueOf(a)
		return a_slice.Len() > 0
	}

	return false
}

type _AddDict struct{}

func (self _AddDict) Applicable(a types.Any, b types.Any) bool {
	_, a_ok := a.(*ordereddict.Dict)
	_, b_ok := b.(*ordereddict.Dict)
	return a_ok && b_ok
}

func (self _AddDict) Add(scope types.Scope, a types.Any, b types.Any) types.Any {
	a_dict, a_ok := a.(*ordereddict.Dict)
	if !a_ok {
		return &vfilter.Null{}
	}
	b_dict, b_ok := b.(*ordereddict.Dict)
	if !b_ok {
		return &vfilter.Null{}
	}

	res := ordereddict.NewDict()

	for _, i := range a_dict.Items() {
		res.Set(i.Key, i.Value)
	}

	for _, i := range b_dict.Items() {
		res.Update(i.Key, i.Value)
	}

	return res
}

type _SubDict struct{}

func (self _SubDict) Applicable(a types.Any, b types.Any) bool {
	_, a_ok := a.(*ordereddict.Dict)
	_, b_ok := b.(*ordereddict.Dict)
	return a_ok && b_ok
}

// Substituting one dict from the other removes keys from the first
// dict.
func (self _SubDict) Sub(scope types.Scope, a types.Any, b types.Any) types.Any {
	a_dict, a_ok := a.(*ordereddict.Dict)
	if !a_ok {
		return &vfilter.Null{}
	}
	b_dict, b_ok := b.(*ordereddict.Dict)
	if !b_ok {
		return &vfilter.Null{}
	}

	// Copy values to new dict if the key is not present in the b
	// dict.
	res := ordereddict.NewDict()

	for _, i := range a_dict.Items() {
		_, pres := b_dict.Get(i.Key)
		if !pres {
			res.Set(i.Key, i.Value)
		}
	}

	return res
}

type _MulDict struct{}

func (self _MulDict) Applicable(a types.Any, b types.Any) bool {
	_, a_ok := a.(*ordereddict.Dict)
	_, b_ok := b.(*ordereddict.Dict)
	return a_ok && b_ok
}

// Multiplying dicts results in an intersection operation
func (self _MulDict) Mul(scope types.Scope, a types.Any, b types.Any) types.Any {
	a_dict, a_ok := a.(*ordereddict.Dict)
	if !a_ok {
		return &vfilter.Null{}
	}
	b_dict, b_ok := b.(*ordereddict.Dict)
	if !b_ok {
		return &vfilter.Null{}
	}

	// Copy values to new dict if the key is also present in the b
	// dict.
	res := ordereddict.NewDict()

	for _, i := range a_dict.Items() {
		_, pres := b_dict.Get(i.Key)
		if pres {
			res.Set(i.Key, i.Value)
		}
	}

	return res
}

// Handle a map adding with a dict.
type _AddMap struct{}

func (self _AddMap) Applicable(a types.Any, b types.Any) bool {
	a_value := reflect.Indirect(reflect.ValueOf(a))
	if a_value.Kind() != reflect.Map {
		return false
	}

	_, b_ok := b.(*ordereddict.Dict)
	return b_ok
}

func (self _AddMap) Add(scope types.Scope, a types.Any, b types.Any) types.Any {
	ctx := context.Background()
	a_dict := vfilter.RowToDict(ctx, scope, a)
	b_dict := vfilter.RowToDict(ctx, scope, b)

	for _, i := range b_dict.Items() {
		a_dict.Set(i.Key, i.Value)
	}

	return a_dict
}

type _RegexDict struct{}

func (self _RegexDict) Applicable(pattern types.Any, target types.Any) bool {
	switch target.(type) {
	case ordereddict.Dict, *ordereddict.Dict:
		return true
	}

	rt := reflect.TypeOf(target)
	if rt == nil {
		return false
	}
	return rt.Kind() == reflect.Map
}

// Applying a regex on a dict means matching the serialized version of
// the dict.
func (self _RegexDict) Match(scope types.Scope,
	pattern types.Any, target types.Any) bool {
	serialized, _ := json.Marshal(target)
	return scope.Match(pattern, string(serialized))
}

type _LtDict struct{}

func (self _LtDict) Applicable(a, b types.Any) bool {
	_, a_ok := a.(*ordereddict.Dict)
	_, b_ok := b.(*ordereddict.Dict)

	return a_ok || b_ok
}

func (self _LtDict) Lt(scope types.Scope, a types.Any, b types.Any) bool {
	a_dict, a_ok := a.(*ordereddict.Dict)
	b_dict, b_ok := b.(*ordereddict.Dict)

	if !a_ok && b_ok {
		return true
	}

	if !b_ok && a_ok {
		return false
	}

	if !b_ok && !a_ok {
		return false
	}

	return a_dict.Len() < b_dict.Len()

}

func init() {
	vql_subsystem.RegisterProtocol(&_RegexDict{})
	vql_subsystem.RegisterProtocol(&_BoolDict{})
	vql_subsystem.RegisterProtocol(&_AddDict{})
	vql_subsystem.RegisterProtocol(&_SubDict{})
	vql_subsystem.RegisterProtocol(&_MulDict{})
	vql_subsystem.RegisterProtocol(&_AddMap{})
	vql_subsystem.RegisterProtocol(&_LtDict{})
}
