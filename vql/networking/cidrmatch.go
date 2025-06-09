package networking

import (
	"context"
	"net"

	"github.com/Velocidex/ordereddict"
	vql_subsystem "www.velocidex.com/golang/velociraptor/vql"
	vfilter "www.velocidex.com/golang/vfilter"
	"www.velocidex.com/golang/vfilter/arg_parser"
)

type _CIDRContainsArgs struct {
	IP     string   `vfilter:"required,field=ip,doc=An IP address"`
	Ranges []string `vfilter:"required,field=ranges,doc=A list of CIDR notation network ranges"`
}

type _CIDRContains struct{}

func (self _CIDRContains) Call(
	ctx context.Context,
	scope vfilter.Scope,
	args *ordereddict.Dict) vfilter.Any {

	defer vql_subsystem.RegisterMonitor(ctx, "cidr_contains", args)()

	arg := &_CIDRContainsArgs{}
	err := arg_parser.ExtractArgsWithContext(ctx, scope, args, arg)
	if err != nil {
		scope.Log("cidr_contains: %s", err.Error())
		return vfilter.Null{}
	}

	ip := net.ParseIP(arg.IP)
	if ip == nil {
		return false
	}

	for _, rng := range arg.Ranges {
		_, ipNet, err := net.ParseCIDR(rng)
		if err != nil {
			scope.Log("cidr_contains: %v", err)
			return false
		}

		if ipNet.Contains(ip) {
			return true
		}
	}
	return false
}

func (self _CIDRContains) Info(scope vfilter.Scope, type_map *vfilter.TypeMap) *vfilter.FunctionInfo {
	return &vfilter.FunctionInfo{
		Name:    "cidr_contains",
		ArgType: type_map.AddType(scope, &_CIDRContainsArgs{}),
	}
}

func init() {
	vql_subsystem.RegisterFunction(&_CIDRContains{})
}
