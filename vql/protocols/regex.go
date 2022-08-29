package protocols

import (
	"fmt"
	"time"

	vql_subsystem "www.velocidex.com/golang/velociraptor/vql"
	"www.velocidex.com/golang/vfilter"
	"www.velocidex.com/golang/vfilter/types"
)

// Teach VQL to apply a regex to some other types we normally come
// against in VQL queries.
type _GenericRegexProtocol struct{}

func (self _GenericRegexProtocol) Applicable(a vfilter.Any, b vfilter.Any) bool {
	_, a_ok := a.(string)
	if !a_ok {
		return false
	}

	switch b.(type) {
	case time.Time, *time.Time, int, int8, int16, int32, int64,
		uint8, uint16, uint32, uint64, float64:
		return true
	}
	return false
}

func (self _GenericRegexProtocol) Match(scope types.Scope,
	pattern types.Any, target types.Any) bool {
	switch t := target.(type) {
	case time.Time, *time.Time, int, int8, int16, int32, int64,
		uint8, uint16, uint32, uint64, float64:
		return scope.Match(pattern, fmt.Sprintf("%v", t))
	}
	return false
}

func init() {
	vql_subsystem.RegisterProtocol(&_GenericRegexProtocol{})
}
