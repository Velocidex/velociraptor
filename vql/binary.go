// VQL bindings to binary parsing.
package vql

import (
	"www.velocidex.com/golang/velociraptor/binary"
	//utils "www.velocidex.com/golang/velociraptor/testing"
	"www.velocidex.com/golang/vfilter"
)

type _binaryFieldImpl struct{}

func (self _binaryFieldImpl) Applicable(a vfilter.Any, b vfilter.Any) bool {
	_, b_ok := b.(string)
	switch a.(type) {
	case binary.BaseObject, *binary.BaseObject:
		return b_ok
	}
	return false
}

func (self _binaryFieldImpl) Associative(
	scope *vfilter.Scope, a vfilter.Any, b vfilter.Any) (vfilter.Any, bool) {
	field := b.(string)

	var res binary.Object

	switch t := a.(type) {
	case binary.BaseObject:
		res = t.Get(field)
	case *binary.BaseObject:
		res = t.Get(field)
	default:
		return nil, false
	}

	// If the resolving returns an error object we have not
	// properly resolved the field.
	_, ok := res.(*binary.ErrorObject)
	if ok {
		return nil, false
	}

	return res.Value(), true
}

func (self _binaryFieldImpl) GetMembers(scope *vfilter.Scope, a vfilter.Any) []string {
	switch t := a.(type) {
	case binary.BaseObject:
		return t.Fields()
	case *binary.BaseObject:
		return t.Fields()
	default:
		return []string{}
	}
}

func init() {
	exportedProtocolImpl = append(exportedProtocolImpl, &_binaryFieldImpl{})
}
