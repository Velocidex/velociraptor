package tools

import (
	"context"
	"net"

	"github.com/Velocidex/ordereddict"
	"github.com/oschwald/maxminddb-golang"
	vql_subsystem "www.velocidex.com/golang/velociraptor/vql"
	"www.velocidex.com/golang/vfilter"
	"www.velocidex.com/golang/vfilter/arg_parser"
)

const (
	geoIPHandle = "$GeoIPDB"
)

type GeoIPFunctionArgs struct {
	IP       string `vfilter:"required,field=ip,doc=IP Address to lookup."`
	Database string `vfilter:"required,field=db,doc=Path to the MaxMind GeoIP Database."`
}

type GeoIPFunction struct{}

func (self GeoIPFunction) Call(
	ctx context.Context,
	scope vfilter.Scope,
	args *ordereddict.Dict) vfilter.Any {

	defer vql_subsystem.RegisterMonitor(ctx, "geoip", args)()

	arg := &GeoIPFunctionArgs{}
	err := arg_parser.ExtractArgsWithContext(ctx, scope, args, arg)
	if err != nil {
		scope.Log("geoip: %v", err)
		return vfilter.Null{}
	}

	var db *maxminddb.Reader

	// Cache key based on the database name.
	key := geoIPHandle + arg.Database
	cached := vql_subsystem.CacheGet(scope, key)
	switch t := cached.(type) {

	case error:
		return vfilter.Null{}

	case nil:
		db, err = maxminddb.Open(arg.Database)
		if err != nil {
			scope.Log("geoip: %v", err)
			// Cache failures for next lookup.
			vql_subsystem.CacheSet(scope, key, err)
			return vfilter.Null{}
		}
		// Attach the database to the root destructor since it
		// does not need to change very often.
		err := vql_subsystem.GetRootScope(scope).AddDestructor(func() {
			db.Close()
		})
		if err != nil {
			scope.Log("geoip: %v", err)
		}

		vql_subsystem.CacheSet(scope, key, db)

	case *maxminddb.Reader:
		db = t

	default:
		// Unexpected value in cache.
		return vfilter.Null{}
	}

	ip := net.ParseIP(arg.IP)
	if ip == nil {
		scope.Log("geoip: invalid IP %v", arg.IP)
		return vfilter.Null{}
	}

	var record interface{}
	err = db.Lookup(ip, &record)
	if err != nil {
		scope.Log("geoip: %v", err)
		return vfilter.Null{}
	}
	return record
}

func (self GeoIPFunction) Info(scope vfilter.Scope, type_map *vfilter.TypeMap) *vfilter.FunctionInfo {
	return &vfilter.FunctionInfo{
		Name:    "geoip",
		Doc:     "Lookup an IP Address using the MaxMind GeoIP database.",
		ArgType: type_map.AddType(scope, &GeoIPFunctionArgs{}),
		Version: 1,
	}
}

func init() {
	vql_subsystem.RegisterFunction(&GeoIPFunction{})
}
