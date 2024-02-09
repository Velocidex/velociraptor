//go:build !windows
// +build !windows

package darwin

import (
	"context"

	"github.com/Velocidex/ordereddict"
	"github.com/ivaxer/go-xattr"
	"www.velocidex.com/golang/velociraptor/accessors"
	"www.velocidex.com/golang/velociraptor/acls"
	vql_subsystem "www.velocidex.com/golang/velociraptor/vql"
	"www.velocidex.com/golang/vfilter"
	"www.velocidex.com/golang/vfilter/arg_parser"
)

type XAttrArgs struct {
	Filename   *accessors.OSPath `vfilter:"required,field=filename,doc=Filename to inspect."`
	Attributes []string          `vfilter:"optional,field=attribute,doc=Attribute to collect."`
	Accessor   string            `vfilter:"optional,field=accessor,doc=File accessor"`
}

type XAttrFunction struct{}

func (self XAttrFunction) Call(
	ctx context.Context,
	scope vfilter.Scope,
	args *ordereddict.Dict) vfilter.Any {
	defer vql_subsystem.CheckForPanic(scope, "xattr")
	attr := map[string]string{}
	arg := &XAttrArgs{}
	err := arg_parser.ExtractArgsWithContext(ctx, scope, args, arg)
	if err != nil {
		scope.Log("xattr: Arg parser: %s", err)
		return nil
	}

	err = vql_subsystem.CheckFilesystemAccess(scope, arg.Accessor)
	if err != nil {
		scope.Log("xattr: %s", err)
		return nil
	}

	filename, err := accessors.GetUnderlyingAPIFilename(arg.Accessor, scope, arg.Filename)
	if err != nil {
		scope.Log("xattr: Failed to get underlying filename for %s: %s", arg.Filename.String(), err)
	}

	if len(arg.Attributes) > 0 {
		attr = self.getAttributeValues(scope, arg.Attributes, filename)
	} else {
		attributes, err := xattr.List(filename)
		if err != nil {
			scope.Log("xattr: Failed to list attributes for filename %s: %s", filename, err)
			return vfilter.Null{}
		}

		attr = self.getAttributeValues(scope, attributes, filename)
	}

	if attr == nil {
		return vfilter.Null{}
	}

	return ordereddict.NewDict().
		Set("filename", filename).
		Set("attribute", attr)
}

func (self *XAttrFunction) getAttributeValues(scope vfilter.Scope, Attributes []string, Filename string) map[string]string {
	ret := map[string]string{}
	for _, attr := range Attributes {
		value, err := xattr.Get(Filename, attr)
		if err != nil {
			continue
		}
		ret[attr] = string(value)
	}
	if len(ret) == 0 {
		scope.Log("xattr: Failed to get attribute values for filename %s", Filename)
		return nil
	}
	return ret
}

func (self XAttrFunction) Info(scope vfilter.Scope, type_map *vfilter.TypeMap) *vfilter.FunctionInfo {
	return &vfilter.FunctionInfo{
		Name:     "xattr",
		Doc:      "query a file for the specified extended attribute",
		ArgType:  type_map.AddType(scope, &XAttrArgs{}),
		Metadata: vql_subsystem.VQLMetadata().Permissions(acls.FILESYSTEM_READ).Build(),
	}
}

func init() {
	vql_subsystem.RegisterFunction(&XAttrFunction{})
}
