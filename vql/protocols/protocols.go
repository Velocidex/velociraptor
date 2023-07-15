package protocols

import (
	"context"
	"time"

	vql_subsystem "www.velocidex.com/golang/velociraptor/vql"
	"www.velocidex.com/golang/vfilter"
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

func init() {
	vql_subsystem.RegisterProtocol(&_BoolTime{})
	vql_subsystem.RegisterProtocol(&_BoolEq{})
}
