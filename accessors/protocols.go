package accessors

import (
	"context"

	"www.velocidex.com/golang/velociraptor/utils"
	vql_subsystem "www.velocidex.com/golang/velociraptor/vql"
	"www.velocidex.com/golang/vfilter"
	"www.velocidex.com/golang/vfilter/protocols"
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
	case *OSPath, string, []vfilter.Any:
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

		// Support OSPath("/root") + ["foo", "bar"] -> OSPath("/root/foo/bar")
	case []vfilter.Any:
		components := make([]string, 0, len(t))
		for _, item := range t {
			str_item, ok := item.(string)
			if ok {
				components = append(components, str_item)
			}
		}

		return a_os_path.Append(components...)
	}
	return a_os_path.Append(b_os_path.Components...)
}

type _AssociativeOSPath struct{}

// Filter some method calls to be more useful.
func (self _AssociativeOSPath) Applicable(a vfilter.Any, b vfilter.Any) bool {
	_, ok := a.(*OSPath)
	if !ok {
		return false
	}
	switch b.(type) {
	case []*int64, string, int64:
		return true
	}
	return false
}

func (self _AssociativeOSPath) Associative(
	scope vfilter.Scope, a vfilter.Any, b vfilter.Any) (vfilter.Any, bool) {
	a_os_path, ok := a.(*OSPath)
	if !ok {
		return &vfilter.Null{}, false
	}

	length := int64(len(a_os_path.Components))

	switch t := b.(type) {
	case []*int64:
		first_item := int64(0)
		if t[0] != nil {
			first_item = *t[0]
		}

		second_item := length
		if t[1] != nil {
			second_item = *t[1]
		}

		if first_item < 0 {
			first_item += length
		}

		if second_item < 0 {
			second_item += length
		}

		if second_item <= first_item {
			return a_os_path.Clear(), true
		}

		return a_os_path.Clear().Append(
			a_os_path.Components[first_item:second_item]...), true

	case int64:
		if t < 0 {
			t += length
		}
		if t < 0 || t > length {
			return &vfilter.Null{}, true
		}
		return a_os_path.Components[t], true

	case string:
		switch t {
		case "HumanString":
			return a_os_path.HumanString(scope), true

		default:
			return protocols.DefaultAssociative{}.Associative(scope, a, b)
		}

	default:
		return protocols.DefaultAssociative{}.Associative(scope, a, b)
	}
}

func (self _AssociativeOSPath) GetMembers(scope vfilter.Scope, a vfilter.Any) []string {
	result := protocols.DefaultAssociative{}.GetMembers(scope, a)
	result = append(result, "HumanString")
	return result
}

func init() {
	vql_subsystem.RegisterProtocol(&_BoolOSPath{})
	vql_subsystem.RegisterProtocol(&_EqualOSPath{})
	vql_subsystem.RegisterProtocol(&_AddOSPath{})
	vql_subsystem.RegisterProtocol(&_RegexOSPath{})
	vql_subsystem.RegisterProtocol(&_AssociativeOSPath{})
}
