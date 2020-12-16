package utils

import "reflect"

// We need to do this stupid check because Go does not allow
// comparison to nil with interfaces.
func IsNil(v interface{}) bool {
	return v == nil || (reflect.ValueOf(v).Kind() == reflect.Ptr &&
		reflect.ValueOf(v).IsNil())
}
