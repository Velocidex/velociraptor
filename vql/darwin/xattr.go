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
	Attributes []string          `vfilter:"optional,field=attribute,doc=Attribute to collect. "`
	Accessor   string            `vfilter:"optional,field=accessor,doc=File accessor"`
}

type XAttrFunction struct{}

func (self XAttrFunction) Call(
	ctx context.Context,
	scope vfilter.Scope,
	args *ordereddict.Dict) vfilter.Any {
	defer vql_subsystem.CheckForPanic(scope, "xattr")
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

	filename, err := accessors.GetUnderlyingAPIFilename(
		arg.Accessor, scope, arg.Filename)
	if err != nil {
		scope.Log("xattr: Failed to get underlying filename for %s: %s",
			arg.Filename.String(), err)
	}

	if len(arg.Attributes) > 0 {
		return self.getAttributeValues(scope, arg.Attributes, filename)
	} else {
		attributes, err := xattr.List(filename)
		if err != nil {
			scope.Log("xattr: Failed to list attributes for filename %s: %s",
				filename, err)
			return vfilter.Null{}
		}

		return self.getAttributeValues(scope, attributes, filename)
	}
}

func (self *XAttrFunction) getAttributeValues(
	scope vfilter.Scope, Attributes []string,
	Filename string) *ordereddict.Dict {
	ret := ordereddict.NewDict()
	for _, attr := range Attributes {
		value, err := xattr.Get(Filename, attr)
		if err != nil {
			continue
		}
		ret.Set(attr, value)
	}
	return ret
}

func (self XAttrFunction) Info(
	scope vfilter.Scope, type_map *vfilter.TypeMap) *vfilter.FunctionInfo {
	return &vfilter.FunctionInfo{
		Name: "xattr",
		Doc: "Query a file for the specified extended attribute. " +
			"If not attributes are provided, this function will " +
			"return all extended attributes for the file. Please note: " +
			"this API is not reliable, so please provided extended " +
			"attributes where possible. ",
		ArgType:  type_map.AddType(scope, &XAttrArgs{}),
		Metadata: vql_subsystem.VQLMetadata().Permissions(acls.FILESYSTEM_READ).Build(),
	}
}

func init() {
	vql_subsystem.RegisterFunction(&XAttrFunction{})
}
