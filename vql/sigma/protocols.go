package sigma

import (
	vql_subsystem "www.velocidex.com/golang/velociraptor/vql"
	"www.velocidex.com/golang/velociraptor/vql/sigma/evaluator"
	"www.velocidex.com/golang/vfilter"
)

type _EventAssociative struct{}

func (self _EventAssociative) Applicable(a vfilter.Any, b vfilter.Any) bool {
	_, a_ok := a.(*evaluator.Event)
	_, b_ok := b.(string)
	return a_ok && b_ok
}

func (self _EventAssociative) GetMembers(scope vfilter.Scope, a vfilter.Any) []string {
	event, event_ok := a.(*evaluator.Event)
	if !event_ok {
		return nil
	}

	return scope.GetMembers(event.Dict)
}

func (self _EventAssociative) Associative(scope vfilter.Scope, a vfilter.Any, b vfilter.Any) (
	vfilter.Any, bool) {
	event, event_ok := a.(*evaluator.Event)
	if !event_ok {
		return nil, false
	}

	return scope.Associative(event.Dict, b)
}

func init() {
	vql_subsystem.RegisterProtocol(&_EventAssociative{})
}
