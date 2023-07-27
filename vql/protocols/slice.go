package protocols

import (
	"reflect"

	vql_subsystem "www.velocidex.com/golang/velociraptor/vql"
	"www.velocidex.com/golang/vfilter"
)

// Comparing slices means to compare their first element
type _SliceLt struct{}

func (self _SliceLt) Applicable(a vfilter.Any, b vfilter.Any) bool {
	a_value := reflect.Indirect(reflect.ValueOf(a))
	b_value := reflect.Indirect(reflect.ValueOf(b))

	return a_value.Type().Kind() == reflect.Slice &&
		b_value.Type().Kind() == reflect.Slice
}

func (self _SliceLt) Lt(scope vfilter.Scope, a vfilter.Any, b vfilter.Any) bool {
	a_value := reflect.Indirect(reflect.ValueOf(a))
	b_value := reflect.Indirect(reflect.ValueOf(b))

	// LHS is empty this has to be less than RHS
	if a_value.Len() == 0 {
		return true
	}

	// RHS is emptry this has to be greater than LHS
	if b_value.Len() == 0 {
		return false
	}

	a0_value := reflect.Indirect(a_value.Index(0)).Interface()
	b0_value := reflect.Indirect(b_value.Index(0)).Interface()

	return scope.Lt(a0_value, b0_value)
}

func init() {
	vql_subsystem.RegisterProtocol(&_SliceLt{})
}
