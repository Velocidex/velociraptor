//go:build !windows
// +build !windows

package linux

// Compatibility with windows - create a passthrough lookupSID()

import (
	"context"

	"github.com/Velocidex/ordereddict"
	"www.velocidex.com/golang/velociraptor/acls"
	"www.velocidex.com/golang/velociraptor/vql"
	vql_subsystem "www.velocidex.com/golang/velociraptor/vql"
	"www.velocidex.com/golang/vfilter"
)

type LookupSidFunctionArgs struct {
	Sid string `vfilter:"required,field=sid,doc=A SID to lookup using LookupAccountSid "`
}

func init() {
	vql_subsystem.RegisterFunction(
		vfilter.GenericFunction{
			ArgType:      &LookupSidFunctionArgs{},
			FunctionName: "lookupSID",
			Metadata:     vql.VQLMetadata().Permissions(acls.MACHINE_STATE).Build(),
			Function: func(
				ctx context.Context,
				scope vfilter.Scope,
				args *ordereddict.Dict) vfilter.Any {

				sid, pres := args.Get("sid")
				if pres {
					return sid
				}
				return &vfilter.Null{}
			},
		})
}
