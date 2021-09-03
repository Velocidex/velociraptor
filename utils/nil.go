package utils

import (
	"reflect"

	"www.velocidex.com/golang/vfilter/types"
)

// We need to do this stupid check because Go does not allow
// comparison to nil with interfaces.
func IsNil(v interface{}) bool {
	switch v.(type) {
	case types.Null, *types.Null:
		return true
	default:
		return v == nil || (reflect.ValueOf(v).Kind() == reflect.Ptr &&
			reflect.ValueOf(v).IsNil())
	}
}
