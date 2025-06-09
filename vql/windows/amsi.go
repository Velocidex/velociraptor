//go:build windows
// +build windows

package windows

import (
	"context"

	"github.com/Velocidex/amsi"
	"github.com/Velocidex/ordereddict"
	vql_subsystem "www.velocidex.com/golang/velociraptor/vql"
	vfilter "www.velocidex.com/golang/vfilter"
	"www.velocidex.com/golang/vfilter/arg_parser"
)

const (
	AMSI_KEY = "$AMSI"
)

type _AMSIFunctionArgs struct {
	String string `vfilter:"required,field=string,doc=A string to scan"`
}

type _AMSIFunction struct{}

func (self _AMSIFunction) Call(
	ctx context.Context,
	scope vfilter.Scope,
	args *ordereddict.Dict) vfilter.Any {

	defer vql_subsystem.RegisterMonitor(ctx, "amsi", args)()

	arg := &_AMSIFunctionArgs{}
	err := arg_parser.ExtractArgsWithContext(ctx, scope, args, arg)
	if err != nil {
		scope.Log("amsi: %v", err)
		return vfilter.Null{}
	}

	// Cache the session across the query context.
	session_any := vql_subsystem.CacheGet(scope, AMSI_KEY)
	if session_any == nil {
		err := amsi.Initialize()
		if err != nil {
			scope.Log("amsi: %v", err)
			return vfilter.Null{}
		}
		session := amsi.OpenSession()

		// Tear it all down when the scope is destroyed.
		vql_subsystem.GetRootScope(scope).AddDestructor(func() {
			amsi.CloseSession(session)
			amsi.Uninitialize()
		})
		vql_subsystem.CacheSet(scope, AMSI_KEY, session)
		session_any = session
	}

	session, ok := session_any.(*amsi.Session)
	if !ok {
		scope.Log("amsi: %v", err)
		return vfilter.Null{}
	}

	result := session.ScanString(arg.String)
	switch result {
	case amsi.ResultClean:
		return "ResultClean"
	case amsi.ResultNotDetected:
		return "ResultNotDetected"
	case amsi.ResultBlockedByAdminStart:
		return "ResultBlockedByAdminStart"
	case amsi.ResultBlockedByAdminEnd:
		return "ResultBlockedByAdminEnd"
	case amsi.ResultDetected:
		return "ResultDetected"
	}

	return vfilter.Null{}
}

func (self _AMSIFunction) Info(scope vfilter.Scope, type_map *vfilter.TypeMap) *vfilter.FunctionInfo {
	return &vfilter.FunctionInfo{
		Name:    "amsi",
		ArgType: type_map.AddType(scope, &_AMSIFunctionArgs{}),
	}
}

func init() {
	vql_subsystem.RegisterFunction(&_AMSIFunction{})
}
