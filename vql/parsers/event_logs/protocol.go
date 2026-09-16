package event_logs

import (
	"www.velocidex.com/golang/evtx"
	vql_subsystem "www.velocidex.com/golang/velociraptor/vql"
	"www.velocidex.com/golang/vfilter/types"
	"www.velocidex.com/golang/vfilter/utils"
)

// The EVTX parser emits a HexInt type for integers to allow them to
// be encoded as hex strings. But they may still need to be
// compareable with an integer.
type _HexIntEq struct{}

func (self _HexIntEq) Eq(scope types.Scope, a types.Any, b types.Any) bool {
	a_value, a_ok := a.(evtx.HexInt)
	if !a_ok {
		return false
	}

	b_value, ok := utils.ToInt64(b)
	if !ok {
		return false
	}

	return b_value == int64(a_value)
}

func (self _HexIntEq) Applicable(a types.Any, b types.Any) bool {
	_, a_ok := a.(evtx.HexInt)
	if !a_ok {
		return false
	}

	return true
}

func init() {
	vql_subsystem.RegisterProtocol(&_HexIntEq{})
}
