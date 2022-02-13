package accessors

import (
	"context"

	"www.velocidex.com/golang/velociraptor/utils"
	vql_subsystem "www.velocidex.com/golang/velociraptor/vql"
	"www.velocidex.com/golang/vfilter"
)

type _BoolOSPath struct{}

func (self _BoolOSPath) Applicable(a vfilter.Any) bool {
	_, ok := a.(*OSPath)
	return ok
}

func (self _BoolOSPath) Bool(ctx context.Context, scope vfilter.Scope, a vfilter.Any) bool {
	os_path, ok := a.(*OSPath)
	return ok && len(os_path.Components) > 0
}

type _EqualOSPath struct{}

func (self _EqualOSPath) Applicable(a vfilter.Any, b vfilter.Any) bool {
	_, ok := a.(*OSPath)
	if !ok {
		return false
	}
	_, ok = b.(*OSPath)
	return ok
}

func (self _EqualOSPath) Eq(scope vfilter.Scope, a vfilter.Any, b vfilter.Any) bool {
	a_os_path, ok := a.(*OSPath)
	if !ok {
		return false
	}

	b_os_path, ok := b.(*OSPath)
	if !ok {
		return false
	}

	if !utils.StringSliceEq(a_os_path.Components, b_os_path.Components) {
		return false
	}

	return a_os_path.String() == b_os_path.String()
}

type _RegexOSPath struct{}

func (self _RegexOSPath) Applicable(a vfilter.Any, b vfilter.Any) bool {
	_, ok := b.(*OSPath)
	if !ok {
		return false
	}
	_, ok = a.(string)
	return ok
}

func (self _RegexOSPath) Match(scope vfilter.Scope, a vfilter.Any, b vfilter.Any) bool {
	b_os_path, ok := b.(*OSPath)
	if !ok {
		return false
	}

	return scope.Match(a, b_os_path.String())
}

type _AddOSPath struct{}

func (self _AddOSPath) Applicable(a vfilter.Any, b vfilter.Any) bool {
	_, ok := a.(*OSPath)
	if !ok {
		return false
	}
	switch b.(type) {
	case *OSPath, string:
		return true
	}
	return false
}

func (self _AddOSPath) Add(scope vfilter.Scope, a vfilter.Any, b vfilter.Any) vfilter.Any {
	a_os_path, ok := a.(*OSPath)
	if !ok {
		return false
	}

	var b_os_path *OSPath

	switch t := b.(type) {
	case *OSPath:
		b_os_path = t
	case string:
		parsed, err := ParsePath(t, "")
		if err != nil {
			return vfilter.Null{}
		}
		b_os_path = parsed
	}
	return a_os_path.Append(b_os_path.Components...)
}

func init() {
	vql_subsystem.RegisterProtocol(&_BoolOSPath{})
	vql_subsystem.RegisterProtocol(&_EqualOSPath{})
	vql_subsystem.RegisterProtocol(&_AddOSPath{})
	vql_subsystem.RegisterProtocol(&_RegexOSPath{})
}
