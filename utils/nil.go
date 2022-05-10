package utils

import (
	"reflect"
	"runtime"

	"www.velocidex.com/golang/vfilter/types"
)

// We need to do this stupid check because Go does not allow
// comparison to nil with interfaces.
func IsNil(v interface{}) bool {
	if v == nil {
		return true
	}

	switch v.(type) {
	case types.Null, *types.Null:
		return true
	}

	switch reflect.TypeOf(v).Kind() {
	case reflect.Ptr, reflect.Map, reflect.Chan, reflect.Slice:
		//use of IsNil method
		return reflect.ValueOf(v).IsNil()

	default:
		return false
	}
}

// Compare two functions by name. This allows setting a constant func
// as a parameter.
func CompareFuncs(a func(), b func()) bool {
	if a == nil || b == nil {
		return false
	}

	name_a := runtime.FuncForPC(reflect.ValueOf(a).Pointer()).Name()
	name_b := runtime.FuncForPC(reflect.ValueOf(b).Pointer()).Name()

	return name_a == name_b
}
