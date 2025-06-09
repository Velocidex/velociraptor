package crypto

import (
	"context"
	"crypto/x509"

	"github.com/Velocidex/ordereddict"
	"github.com/Velocidex/pkcs7"
	"www.velocidex.com/golang/go-pe"
	"www.velocidex.com/golang/vfilter"
	"www.velocidex.com/golang/vfilter/arg_parser"

	vql_subsystem "www.velocidex.com/golang/velociraptor/vql"
)

type ParsePKCS7FunctionArg struct {
	Data string `vfilter:"required,field=data,doc=PKCS7 DER encoded string."`
}
type ParsePKCS7Function struct{}

func (self ParsePKCS7Function) Info(scope vfilter.Scope, type_map *vfilter.TypeMap) *vfilter.FunctionInfo {
	return &vfilter.FunctionInfo{
		Name:    "parse_pkcs7",
		Doc:     "Parse a DER encoded pkcs7 string into an object.",
		ArgType: type_map.AddType(scope, &ParsePKCS7FunctionArg{}),
	}
}

func (self ParsePKCS7Function) Call(
	ctx context.Context, scope vfilter.Scope,
	args *ordereddict.Dict) vfilter.Any {

	defer vql_subsystem.RegisterMonitor(ctx, "parse_pkcs7", args)()

	arg := &ParsePKCS7FunctionArg{}
	err := arg_parser.ExtractArgsWithContext(ctx, scope, args, arg)
	if err != nil {
		scope.Log("parse_pkcs7: %v", err)
		return &vfilter.Null{}
	}

	pkcs7_obj, err := pkcs7.Parse([]byte(arg.Data))
	if err != nil {
		scope.Log("parse_pkcs7: %v", err)
		return &vfilter.Null{}
	}

	return pe.PKCS7ToOrderedDict(pkcs7_obj)
}

type ParseX509FunctionArg struct {
	Data string `vfilter:"required,field=data,doc=X509 DER encoded string."`
}
type ParseX509Function struct{}

func (self ParseX509Function) Info(scope vfilter.Scope, type_map *vfilter.TypeMap) *vfilter.FunctionInfo {
	return &vfilter.FunctionInfo{
		Name:    "parse_x509",
		Doc:     "Parse a DER encoded x509 string into an object.",
		ArgType: type_map.AddType(scope, &ParseX509FunctionArg{}),
	}
}

func (self ParseX509Function) Call(
	ctx context.Context, scope vfilter.Scope,
	args *ordereddict.Dict) vfilter.Any {

	defer vql_subsystem.RegisterMonitor(ctx, "parse_x509", args)()

	arg := &ParseX509FunctionArg{}
	err := arg_parser.ExtractArgsWithContext(ctx, scope, args, arg)
	if err != nil {
		scope.Log("parse_x509: %v", err)
		return &vfilter.Null{}
	}

	x509_obj, err := x509.ParseCertificates([]byte(arg.Data))
	if err != nil {
		scope.Log("parse_x509: %v", err)
		return &vfilter.Null{}
	}

	var result []*ordereddict.Dict
	for _, cert := range x509_obj {
		result = append(result, pe.X509ToOrderedDict(cert))
	}

	return result
}

func init() {
	vql_subsystem.RegisterFunction(&ParsePKCS7Function{})
	vql_subsystem.RegisterFunction(&ParseX509Function{})
}
