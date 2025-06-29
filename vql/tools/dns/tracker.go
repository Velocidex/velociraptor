package dns

import (
	"context"
	"time"

	"github.com/Velocidex/ordereddict"
	"github.com/Velocidex/ttlcache/v2"
	vql_subsystem "www.velocidex.com/golang/velociraptor/vql"
	"www.velocidex.com/golang/vfilter"
	"www.velocidex.com/golang/vfilter/arg_parser"
)

/*
The DNS tracker is used to keep track of DNS name to IP
relationships.

It is basically a DNS cache but it can be populated from external
events.
*/
var (
	DNSCache = NewDNSTracker()
)

type DNSRecord struct {
	IP   string
	Name string
}

func NewDNSTracker() *DNSTracker {
	res := &DNSTracker{
		// map[string]map[string]bool
		by_name: ttlcache.NewCache(),
		by_ip:   ttlcache.NewCache(),
	}

	_ = res.by_ip.SetTTL(time.Hour)
	_ = res.by_name.SetTTL(time.Hour)
	res.by_ip.SetCacheSizeLimit(1000)
	res.by_name.SetCacheSizeLimit(1000)

	return res
}

type DNSTracker struct {
	by_ip   *ttlcache.Cache
	by_name *ttlcache.Cache
}

func (self *DNSTracker) Set(ip, name string) {
	set_any, err := self.by_ip.Get(ip)
	if err != nil {
		set_any = make(map[string]bool)
	}

	set, ok := set_any.(map[string]bool)
	if ok {
		set[name] = true
	}
	_ = self.by_ip.Set(ip, set)

	set_any, err = self.by_name.Get(name)
	if err != nil {
		set_any = make(map[string]bool)
	}

	set, ok = set_any.(map[string]bool)
	if ok {
		set[ip] = true
	}
	_ = self.by_name.Set(name, set)
}

func (self *DNSTracker) ByName(name string) (res []string) {
	return self.getLRU(name, self.by_name)
}

func (self *DNSTracker) ByIP(ip string) (res []string) {
	return self.getLRU(ip, self.by_ip)
}

func (self *DNSTracker) getLRU(name string, lru *ttlcache.Cache) (res []string) {
	set_any, err := lru.Get(name)
	if err != nil {
		return nil
	}

	set, ok := set_any.(map[string]bool)
	if ok {
		for k := range set {
			res = append(res, k)
		}
	}

	return res
}

type CacheDNSArgs struct {
	Name string `vfilter:"required,field=name,doc=The domain name to cache "`
	IP   string `vfilter:"required,field=ip,doc=The ip of the domain"`
}

type CacheDNS struct{}

func (self CacheDNS) Call(ctx context.Context,
	scope vfilter.Scope,
	args *ordereddict.Dict) vfilter.Any {

	defer vql_subsystem.RegisterMonitor(ctx, "cache_dns", args)()

	arg := &CacheDNSArgs{}
	err := arg_parser.ExtractArgsWithContext(ctx, scope, args, arg)
	if err != nil {
		scope.Log("cache_dns: %v", err)
		return false
	}

	DNSCache.Set(arg.Name, arg.IP)

	return true
}

func (self *CacheDNS) Info(scope vfilter.Scope, type_map *vfilter.TypeMap) *vfilter.FunctionInfo {
	return &vfilter.FunctionInfo{
		Name:    "cache_dns",
		Doc:     "Add a DNS record to the cache..",
		ArgType: type_map.AddType(scope, &CacheDNSArgs{}),
	}
}

func init() {
	vql_subsystem.RegisterFunction(&CacheDNS{})
}
