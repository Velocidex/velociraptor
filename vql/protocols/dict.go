package protocols

import (
	"context"
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

	for _, k := range a_dict.Keys() {
		v, _ := a_dict.Get(k)
		res.Set(k, v)
	}

	for _, k := range b_dict.Keys() {
		v, _ := b_dict.Get(k)
		res.Set(k, v)
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

	for _, k := range b_dict.Keys() {
		v, _ := b_dict.Get(k)
		a_dict.Set(k, v)
	}

	return a_dict
}

func init() {
	vql_subsystem.RegisterProtocol(&_BoolDict{})
	vql_subsystem.RegisterProtocol(&_AddDict{})
	vql_subsystem.RegisterProtocol(&_AddMap{})
}
