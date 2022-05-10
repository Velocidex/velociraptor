package protocols

import (
	"context"
	"time"

	"www.velocidex.com/golang/velociraptor/accessors"
	vql_subsystem "www.velocidex.com/golang/velociraptor/vql"
	"www.velocidex.com/golang/vfilter"
	"www.velocidex.com/golang/vfilter/protocols"
	"www.velocidex.com/golang/vfilter/types"
)

type _BoolTime struct{}

func (self _BoolTime) Applicable(a vfilter.Any) bool {
	switch a.(type) {
	case time.Time, *time.Time:
		return true
	}
	return false
}

func (self _BoolTime) Bool(ctx context.Context, scope vfilter.Scope, a vfilter.Any) bool {
	switch t := a.(type) {
	case time.Time:
		return t.Unix() > 0
	case *time.Time:
		return t.Unix() > 0
	}

	return false
}

type _BoolEq struct{}

func (self _BoolEq) Eq(scope types.Scope, a types.Any, b types.Any) bool {
	b_value := false
	switch t := b.(type) {
	case string:
		switch t {
		case "Y", "y", "TRUE", "True":
			b_value = true
		}
	case bool:
		b_value = t
	}

	return scope.Bool(a) == b_value
}

func (self _BoolEq) Applicable(a types.Any, b types.Any) bool {
	_, a_ok := a.(bool)
	if !a_ok {
		return false
	}

	switch b.(type) {
	case string, bool:
		return true
	}

	return false
}

type _GlobFileInfoAssociative struct{}

func (self _GlobFileInfoAssociative) Applicable(
	a vfilter.Any, b vfilter.Any) bool {
	_, a_ok := a.(accessors.FileInfo)
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
	scope vfilter.Scope, a vfilter.Any, b vfilter.Any) (
	vfilter.Any, bool) {
	return protocols.DefaultAssociative{}.Associative(scope, a, b)
}

// Only expose some fields that are explicitly provided by the
// glob.FileInfo interface. This cleans up * expansion in SELECT *
// FROM ...
func (self _GlobFileInfoAssociative) GetMembers(
	scope vfilter.Scope, a vfilter.Any) []string {
	return []string{"Name", "ModTime", "FullPath",
		"OSPath", "Mtime", "Btime",
		"Ctime", "Atime", "Data", "Size",
		"IsDir", "IsLink", "Mode"}
}

func init() {
	vql_subsystem.RegisterProtocol(&_BoolTime{})
	vql_subsystem.RegisterProtocol(&_BoolEq{})
	vql_subsystem.RegisterProtocol(&_GlobFileInfoAssociative{})
}
