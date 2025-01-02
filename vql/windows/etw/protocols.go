//go:build windows && cgo && amd64
// +build windows,cgo,amd64

package etw

import (
	"github.com/Velocidex/etw"
	vql_subsystem "www.velocidex.com/golang/velociraptor/vql"
	"www.velocidex.com/golang/vfilter"
	"www.velocidex.com/golang/vfilter/protocols"
)

type etwEventAssociative struct{}

func (self etwEventAssociative) Applicable(a vfilter.Any, b vfilter.Any) bool {
	_, a_ok := a.(*etw.Event)
	if !a_ok {
		return false
	}

	_, b_ok := b.(string)
	return b_ok
}
func (self etwEventAssociative) GetMembers(scope vfilter.Scope, a vfilter.Any) []string {
	_, a_ok := a.(*etw.Event)
	if a_ok {
		return []string{"System", "ProviderGUID", "EventData", "Backtrace"}
	}
	return []string{}
}

func (self etwEventAssociative) Associative(
	scope vfilter.Scope, a vfilter.Any, b vfilter.Any) (vfilter.Any, bool) {
	a_value, a_ok := a.(*etw.Event)
	if !a_ok {
		return vfilter.Null{}, false
	}

	b_value, b_ok := b.(string)
	if !b_ok {
		return vfilter.Null{}, false
	}

	switch b_value {
	case "System":
		return a_value.HeaderProps(), true
	case "ProviderGUID":
		return a_value.Header.ProviderID.String(), true
	case "EventData":
		return a_value.Props(), true
	case "Backtrace":
		return a_value.Backtrace(), true
	default:
		return protocols.DefaultAssociative{}.Associative(scope, a, b)
	}
}

func init() {
	vql_subsystem.RegisterProtocol(&etwEventAssociative{})
}
