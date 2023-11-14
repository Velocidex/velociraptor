// +build !windows

package linux

// Compatibility with windows - create a passthrough lookupSID()

import (
	"context"

	"github.com/Velocidex/ordereddict"
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
