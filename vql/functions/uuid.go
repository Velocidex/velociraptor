package functions

import (
	"context"

	"github.com/Velocidex/ordereddict"
	"github.com/google/uuid"
	vql_subsystem "www.velocidex.com/golang/velociraptor/vql"
	"www.velocidex.com/golang/vfilter"
)

type _UUIDFunc struct{}

func (self _UUIDFunc) Info(scope vfilter.Scope, type_map *vfilter.TypeMap) *vfilter.FunctionInfo {
	return &vfilter.FunctionInfo{
		Name: "uuid",
		Doc:  "Generate a UUID.",
	}
}

func (self _UUIDFunc) Call(ctx context.Context, scope vfilter.Scope,
	args *ordereddict.Dict) vfilter.Any {

	return uuid.New()
}

func init() {
	vql_subsystem.RegisterFunction(&_UUIDFunc{})
}
