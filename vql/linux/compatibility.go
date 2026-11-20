//go:build !windows
// +build !windows

package linux

// Compatibility with windows - create a passthrough lookupSID()

import (
	"context"

	"github.com/Velocidex/ordereddict"
	"www.velocidex.com/golang/velociraptor/acls"
	vql_subsystem "www.velocidex.com/golang/velociraptor/vql"
	"www.velocidex.com/golang/vfilter"
)

type LookupSidFunctionArgs struct {
	Sid string `vfilter:"required,field=sid,doc=A SID to lookup using LookupAccountSid "`
}

type LookupSidFunction struct{}

func (self LookupSidFunction) Call(ctx context.Context,
	scope vfilter.Scope,
	args *ordereddict.Dict) vfilter.Any {

	err := vql_subsystem.CheckAccess(scope, acls.MACHINE_STATE)
	if err != nil {
		scope.Log("lookupSID: %s", err)
		return vfilter.Null{}
	}

	sid, pres := args.Get("sid")
	if pres {
		return sid
	}
	return &vfilter.Null{}
}

func (self LookupSidFunction) Info(
	scope vfilter.Scope,
	type_map *vfilter.TypeMap) *vfilter.FunctionInfo {
	return &vfilter.FunctionInfo{
		Name:     "lookupSID",
		Metadata: vql_subsystem.VQLMetadata().Permissions(acls.MACHINE_STATE).Build(),
		ArgType:  type_map.AddType(scope, &LookupSidFunctionArgs{}),
	}
}

func init() {
	vql_subsystem.RegisterFunction(&LookupSidFunction{})
}
