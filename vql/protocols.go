package vql

import (
	"reflect"

	"github.com/Velocidex/ordereddict"
	"www.velocidex.com/golang/vfilter"
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

func (self _BoolDict) Bool(scope *vfilter.Scope, a vfilter.Any) bool {
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

func init() {
	RegisterProtocol(&_BoolDict{})
}
