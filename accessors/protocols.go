package accessors

import (
	"context"
	"reflect"

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
	return ok && (len(os_path.Components) > 0 ||
		os_path.DelegatePath() != "")
}

type _BoolFileInfo struct{}

func (self _BoolFileInfo) Applicable(a vfilter.Any) bool {
	_, ok := a.(FileInfo)
	return ok
}

func (self _BoolFileInfo) Bool(
	ctx context.Context, scope vfilter.Scope, a vfilter.Any) bool {
	return true
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

type _LtOSPath struct{}

func (self _LtOSPath) Applicable(a vfilter.Any, b vfilter.Any) bool {
	_, ok := a.(*OSPath)
	if !ok {
		return false
	}
	_, ok = b.(*OSPath)
	return ok
}

func (self _LtOSPath) Lt(scope vfilter.Scope, a vfilter.Any, b vfilter.Any) bool {
	a_os_path, ok := a.(*OSPath)
	if !ok {
		return false
	}

	b_os_path, ok := b.(*OSPath)
	if !ok {
		return false
	}
	return a_os_path.String() < b_os_path.String()
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

	// Shortcut for matches against "." or an empty string will match
	// anything - we do not need to expand the OSPath
	a_str, ok := a.(string)
	if !ok || a_str == "" || a_str == "." {
		return true
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

	a_value := reflect.Indirect(reflect.ValueOf(b))

	return a_value.Type().Kind() == reflect.Slice
}

func (self _AddOSPath) Add(scope vfilter.Scope, a vfilter.Any, b vfilter.Any) vfilter.Any {
	a_os_path, ok := a.(*OSPath)
	if !ok {
		return false
	}

	switch t := b.(type) {
	case *OSPath:
		return a_os_path.Append(t.Components...)

	case string:
		parsed, err := ParsePath(t, "")
		if err != nil {
			return vfilter.Null{}
		}
		return a_os_path.Append(parsed.Components...)
	}

	a_value := reflect.Indirect(reflect.ValueOf(b))
	if a_value.Type().Kind() == reflect.Slice {
		components := []string{}
		for idx := 0; idx < a_value.Len(); idx++ {
			item := a_value.Index(int(idx)).Interface()
			str_item, ok := item.(string)
			if ok {
				components = append(components, str_item)
			}
		}

		return a_os_path.Append(components...)
	}

	return a_os_path
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
			if second_item > length {
				second_item = length
			}
		}

		// Wrap around behavior for negative index.
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
		if t < 0 || t >= length {
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
	vql_subsystem.RegisterProtocol(&_BoolFileInfo{})
	vql_subsystem.RegisterProtocol(&_EqualOSPath{})
	vql_subsystem.RegisterProtocol(&_LtOSPath{})
	vql_subsystem.RegisterProtocol(&_AddOSPath{})
	vql_subsystem.RegisterProtocol(&_RegexOSPath{})
	vql_subsystem.RegisterProtocol(&_AssociativeOSPath{})
}
