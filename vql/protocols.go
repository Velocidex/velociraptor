package vql

import (
	"reflect"

	"github.com/Velocidex/ordereddict"
	"www.velocidex.com/golang/velociraptor/glob"
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

type _GlobFileInfoAssociative struct{}

func (self _GlobFileInfoAssociative) Applicable(
	a vfilter.Any, b vfilter.Any) bool {
	_, a_ok := a.(glob.FileInfo)
	if !a_ok {
		return false
	}

	_, b_ok := b.(string)
	if !b_ok {
		return false
	}
	return true
}

func (self _GlobFileInfoAssociative) Associative(
	scope *vfilter.Scope, a vfilter.Any, b vfilter.Any) (
	vfilter.Any, bool) {
	return vfilter.DefaultAssociative{}.Associative(scope, a, b)
}

// Only expose some fields that are explicitly provided by the
// glob.FileInfo interface. This cleans up * expansion in SELECT *
// FROM ...
func (self _GlobFileInfoAssociative) GetMembers(
	scope *vfilter.Scope, a vfilter.Any) []string {
	return []string{"Name", "ModTime", "FullPath", "Mtime",
		"Ctime", "Atime", "Data", "Size",
		"IsDir", "IsLink", "Mode", "Sys"}
}

func init() {
	RegisterProtocol(&_BoolDict{})
	RegisterProtocol(&_GlobFileInfoAssociative{})
}
