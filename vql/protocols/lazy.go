package protocols

// Plugins may return lazy objects by incorporating them in
// callables. This file adds VQL protocols to be able to handle such
// lazy objects transparently.

// A plugin may send a lazy object like:
// output_chan <- ordereddict.NewDict().
//   Set("Foo", func() vfilter.Any {
//        return ....
//   })
//
// The function will only be called when needed thereby save
// work. Within VQL it should be possible to treat the Foo field as
// any other:
//
//  WHERE Foo =~ "hello"   <--- Will call the function and apply the regex to the result.
//

import (
	"context"

	vql_subsystem "www.velocidex.com/golang/velociraptor/vql"
	"www.velocidex.com/golang/vfilter"
)

func callable(v vfilter.Any) (func() vfilter.Any, bool) {
	res, ok := v.(func() vfilter.Any)
	return res, ok
}

type _CallableBool struct{}

func (self _CallableBool) Applicable(a vfilter.Any) bool {
	_, ok := callable(a)
	return ok
}

func (self _CallableBool) Bool(ctx context.Context, scope vfilter.Scope, a vfilter.Any) bool {
	v, _ := callable(a)
	return scope.Bool(v())
}

type _CallableEq struct{}

func (self _CallableEq) Applicable(a vfilter.Any, b vfilter.Any) bool {
	_, a_ok := callable(a)
	_, b_ok := callable(b)
	return a_ok || b_ok
}

func (self _CallableEq) Eq(scope vfilter.Scope, a vfilter.Any, b vfilter.Any) bool {
	a_value, a_ok := callable(a)
	if a_ok {
		return scope.Eq(a_value(), b)
	}
	b_value, b_ok := callable(b)
	if b_ok {
		return scope.Eq(a, b_value())
	}
	return false
}

type _CallableLt struct{}

func (self _CallableLt) Applicable(a vfilter.Any, b vfilter.Any) bool {
	_, a_ok := callable(a)
	_, b_ok := callable(b)
	return a_ok || b_ok
}

func (self _CallableLt) Lt(scope vfilter.Scope, a vfilter.Any, b vfilter.Any) bool {
	a_value, a_ok := callable(a)
	if a_ok {
		return scope.Lt(a_value(), b)
	}
	b_value, b_ok := callable(b)
	if b_ok {
		return scope.Lt(a, b_value())
	}
	return false
}

type _CallableAdd struct{}

func (self _CallableAdd) Applicable(a vfilter.Any, b vfilter.Any) bool {
	_, a_ok := callable(a)
	_, b_ok := callable(b)
	return a_ok || b_ok
}

func (self _CallableAdd) Add(scope vfilter.Scope, a vfilter.Any, b vfilter.Any) vfilter.Any {
	a_value, a_ok := callable(a)
	if a_ok {
		return scope.Add(a_value(), b)
	}
	b_value, b_ok := callable(b)
	if b_ok {
		return scope.Add(a, b_value())
	}
	return vfilter.Null{}
}

type _CallableSub struct{}

func (self _CallableSub) Applicable(a vfilter.Any, b vfilter.Any) bool {
	_, a_ok := callable(a)
	_, b_ok := callable(b)
	return a_ok || b_ok
}

func (self _CallableSub) Sub(scope vfilter.Scope, a vfilter.Any, b vfilter.Any) vfilter.Any {
	a_value, a_ok := callable(a)
	if a_ok {
		return scope.Sub(a_value(), b)
	}
	b_value, b_ok := callable(b)
	if b_ok {
		return scope.Sub(a, b_value())
	}
	return vfilter.Null{}
}

type _CallableMul struct{}

func (self _CallableMul) Applicable(a vfilter.Any, b vfilter.Any) bool {
	_, a_ok := callable(a)
	_, b_ok := callable(b)
	return a_ok || b_ok
}

func (self _CallableMul) Mul(scope vfilter.Scope, a vfilter.Any, b vfilter.Any) vfilter.Any {
	a_value, a_ok := callable(a)
	if a_ok {
		return scope.Mul(a_value(), b)
	}
	b_value, b_ok := callable(b)
	if b_ok {
		return scope.Mul(a, b_value())
	}
	return vfilter.Null{}
}

type _CallableDiv struct{}

func (self _CallableDiv) Applicable(a vfilter.Any, b vfilter.Any) bool {
	_, a_ok := callable(a)
	_, b_ok := callable(b)
	return a_ok || b_ok
}

func (self _CallableDiv) Div(scope vfilter.Scope, a vfilter.Any, b vfilter.Any) vfilter.Any {
	a_value, a_ok := callable(a)
	if a_ok {
		return scope.Div(a_value(), b)
	}
	b_value, b_ok := callable(b)
	if b_ok {
		return scope.Div(a, b_value())
	}
	return vfilter.Null{}
}

type _CallableMembership struct{}

func (self _CallableMembership) Applicable(a vfilter.Any, b vfilter.Any) bool {
	_, a_ok := callable(a)
	_, b_ok := callable(b)
	return a_ok || b_ok
}

func (self _CallableMembership) Membership(scope vfilter.Scope, a vfilter.Any, b vfilter.Any) bool {
	a_value, a_ok := callable(a)
	if a_ok {
		return scope.Membership(a_value(), b)
	}
	b_value, b_ok := callable(b)
	if b_ok {
		return scope.Membership(a, b_value())
	}
	return false
}

type _CallableAssociative struct{}

func (self _CallableAssociative) Applicable(a vfilter.Any, b vfilter.Any) bool {
	_, a_ok := callable(a)
	_, b_ok := callable(b)
	return a_ok || b_ok
}

func (self _CallableAssociative) GetMembers(scope vfilter.Scope, a vfilter.Any) []string {
	a_value, a_ok := callable(a)
	if a_ok {
		return scope.GetMembers(a_value())
	}
	return []string{}
}

func (self _CallableAssociative) Associative(scope vfilter.Scope, a vfilter.Any, b vfilter.Any) (
	vfilter.Any, bool) {
	a_value, a_ok := callable(a)
	if a_ok {
		return scope.Associative(a_value(), b)
	}
	b_value, b_ok := callable(b)
	if b_ok {
		return scope.Associative(a, b_value())
	}
	return vfilter.Null{}, false
}

type _CallableRegex struct{}

func (self _CallableRegex) Applicable(a vfilter.Any, b vfilter.Any) bool {
	_, b_ok := callable(b)
	return b_ok
}

func (self _CallableRegex) Match(scope vfilter.Scope, a vfilter.Any, b vfilter.Any) bool {
	b_value, b_ok := callable(b)
	if b_ok {
		return scope.Match(a, b_value())
	}
	return false
}

type _CallableIterate struct{}

func (self _CallableIterate) Applicable(a vfilter.Any) bool {
	_, a_ok := callable(a)
	return a_ok
}

func (self _CallableIterate) Iterate(ctx context.Context,
	scope vfilter.Scope, a vfilter.Any) <-chan vfilter.Row {

	a_value, a_ok := callable(a)
	if a_ok {
		return scope.Iterate(ctx, a_value())
	}
	output_chan := make(chan vfilter.Row)
	close(output_chan)

	return output_chan
}

func init() {
	vql_subsystem.RegisterProtocol(&_CallableBool{})
	vql_subsystem.RegisterProtocol(&_CallableEq{})
	vql_subsystem.RegisterProtocol(&_CallableLt{})
	vql_subsystem.RegisterProtocol(&_CallableAdd{})
	vql_subsystem.RegisterProtocol(&_CallableSub{})
	vql_subsystem.RegisterProtocol(&_CallableMul{})
	vql_subsystem.RegisterProtocol(&_CallableDiv{})
	vql_subsystem.RegisterProtocol(&_CallableMembership{})
	vql_subsystem.RegisterProtocol(&_CallableAssociative{})
	vql_subsystem.RegisterProtocol(&_CallableRegex{})
	vql_subsystem.RegisterProtocol(&_CallableIterate{})
}
