/*
Velociraptor - Dig Deeper
Copyright (C) 2019-2025 Rapid7 Inc.

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as published
by the Free Software Foundation, either version 3 of the License, or
(at your option) any later version.

This program is distributed in the hope that it will be useful,
but WITHOUT ANY WARRANTY; without even the implied warranty of
MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
GNU Affero General Public License for more details.

You should have received a copy of the GNU Affero General Public License
along with this program.  If not, see <https://www.gnu.org/licenses/>.
*/
package functions

import (
	"context"
	"net"

	"github.com/Velocidex/ordereddict"
	vql_subsystem "www.velocidex.com/golang/velociraptor/vql"
	"www.velocidex.com/golang/vfilter"
	"www.velocidex.com/golang/vfilter/arg_parser"
)

type IpArgs struct {
	Parse      string `vfilter:"optional,field=parse,doc=Parse the IP as an IPv4 or IPv6 address."`
	Netaddr4LE int64  `vfilter:"optional,field=netaddr4_le,doc=A network order IPv4 address (as little endian)."`
	Netaddr4BE int64  `vfilter:"optional,field=netaddr4_be,doc=A network order IPv4 address (as big endian)."`
}

type IpFunction struct{}

func (self *IpFunction) Call(ctx context.Context,
	scope vfilter.Scope,
	args *ordereddict.Dict) vfilter.Any {

	defer vql_subsystem.RegisterMonitor(ctx, "ip", args)()

	arg := &IpArgs{}
	err := arg_parser.ExtractArgsWithContext(ctx, scope, args, arg)
	if err != nil {
		scope.Log("ip: %s", err.Error())
		return false
	}

	to_parse := arg.Parse
	if to_parse == "" {
		if arg.Netaddr4LE > 0 {
			ip := uint32(arg.Netaddr4LE)
			return net.IPv4(
				byte(ip),
				byte(ip>>8),
				byte(ip>>16),
				byte(ip>>24))
		} else {
			ip := uint32(arg.Netaddr4BE)
			return net.IPv4(
				byte(ip>>24),
				byte(ip>>16),
				byte(ip>>8),
				byte(ip))
		}
	}

	ip := net.ParseIP(to_parse)
	if ip == nil {
		return vfilter.Null{}
	}

	return ip
}

func (self IpFunction) Info(scope vfilter.Scope, type_map *vfilter.TypeMap) *vfilter.FunctionInfo {
	return &vfilter.FunctionInfo{
		Name:    "ip",
		Doc:     "Format an IP address.",
		ArgType: type_map.AddType(scope, &IpArgs{}),
	}
}

func init() {
	vql_subsystem.RegisterFunction(&IpFunction{})
}
