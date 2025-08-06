package networking

import (
	"context"
	"errors"
	"net"

	"github.com/Velocidex/ordereddict"
	"www.velocidex.com/golang/velociraptor/acls"
	"www.velocidex.com/golang/velociraptor/vql"
	vql_subsystem "www.velocidex.com/golang/velociraptor/vql"
	"www.velocidex.com/golang/velociraptor/vql/functions"
	"www.velocidex.com/golang/velociraptor/vql/tools/dns"
	vfilter "www.velocidex.com/golang/vfilter"
	"www.velocidex.com/golang/vfilter/arg_parser"
)

type HostFunctionArgs struct {
	Name        string `vfilter:"required,field=name,doc=The name to lookup"`
	Server      string `vfilter:"optional,field=server,doc=A DNS server to query - if not provided uses the system resolver."`
	Type        string `vfilter:"optional,field=type,doc=Type of lookup, can be CNAME, NS, SOA, TXT, DNSKEY, AXFR, A (default)"`
	PreferGo    bool   `vfilter:"optional,field=prefer_go,doc=Prefer calling the native Go implementation rather than the system."`
	TrackerOnly bool   `vfilter:"optional,field=tracker_only,doc=Only use the dns tracker - if the IP is not known then do not attempt to resolve it."`
}

type HostFunction struct{}

func (self *HostFunction) Call(ctx context.Context,
	scope vfilter.Scope,
	args *ordereddict.Dict) vfilter.Any {

	defer vql_subsystem.RegisterMonitor(ctx, "host", args)()

	arg := &HostFunctionArgs{}

	err := vql_subsystem.CheckAccess(scope, acls.NETWORK)
	if err != nil {
		scope.Log("host: %v", err)
		return false
	}

	err = arg_parser.ExtractArgsWithContext(ctx, scope, args, arg)
	if err != nil {
		scope.Log("host: %v", err)
		return false
	}

	resolver := net.Resolver{
		PreferGo: arg.PreferGo,
	}

	// Override connection if we need to.
	if arg.Server != "" {
		// If the user specified a server then we must do the resolving
		// outside the C library.
		resolver.PreferGo = true
		resolver.Dial = func(ctx context.Context,
			network, address string) (net.Conn, error) {
			dialer := net.Dialer{}
			return dialer.DialContext(ctx, network, arg.Server)
		}
	}

	var addresses interface{}

	switch arg.Type {
	case "", "A":
		// Try to get from the DNSResolver
		ips := dns.DNSCache.ByName(arg.Name)
		if len(ips) > 0 {
			return ips
		}

		if arg.TrackerOnly {
			return []string{}
		}

		addresses, err = resolver.LookupHost(ctx, arg.Name)

	case "PTR":
		// Try to get from the DNSResolver
		names := dns.DNSCache.ByIP(arg.Name)
		if len(names) > 0 {
			return names
		}

		if arg.TrackerOnly {
			return []string{}
		}

		addresses, err = resolver.LookupAddr(ctx, arg.Name)

	case "NS":
		addresses, err = resolver.LookupNS(ctx, arg.Name)

	case "MX":
		addresses, err = resolver.LookupMX(ctx, arg.Name)

	case "SRV":
		_, addresses, err = resolver.LookupSRV(ctx, "", "", arg.Name)

	case "TXT":
		addresses, err = resolver.LookupTXT(ctx, arg.Name)

	case "CNAME":
		addresses, err = resolver.LookupCNAME(ctx, arg.Name)

	case "IP":
		addresses, err = resolver.LookupIPAddr(ctx, arg.Name)

	default:
		addresses = vfilter.Null{}
		err = errors.New(
			"Invalid lookup type: should be one of A,PTR,NS,MX,SRV,TXT,CNAME,Port,IP")
	}

	if err != nil {
		functions.DeduplicatedLog(ctx, scope,
			"host: Lookup error "+arg.Type+": %v", err)
	}
	return addresses
}

func (self *HostFunction) Info(scope vfilter.Scope, type_map *vfilter.TypeMap) *vfilter.FunctionInfo {
	return &vfilter.FunctionInfo{
		Name:     "host",
		Doc:      "Perform a DNS resolution.",
		ArgType:  type_map.AddType(scope, &HostFunctionArgs{}),
		Metadata: vql.VQLMetadata().Permissions(acls.MACHINE_STATE).Build(),
	}
}

func init() {
	vql_subsystem.RegisterFunction(&HostFunction{})
}
