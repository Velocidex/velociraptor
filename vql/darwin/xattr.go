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

type XAttrFunction struct {
	Filename   string
	Attributes map[string]string
}

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

	self.Filename, err = accessors.GetUnderlyingAPIFilename(arg.Accessor, scope, arg.Filename)
	self.Attributes = map[string]string{}
	if err != nil {
		scope.Log("xattr: Failed to get underlying filename for %s: %s", arg.Filename.String(), err)
	}

	if len(arg.Attributes) > 0 {
		self.getAttributeValues(scope, arg.Attributes)
	} else {
		attributes, err := xattr.List(self.Filename)
		if err != nil {
			scope.Log("xattr: Failed to list attributes for filename %s: %s", self.Filename, err)
			return nil
		}

		self.getAttributeValues(scope, attributes)
	}

	if len(self.Attributes) == 0 {
		return nil
	}

	return ordereddict.NewDict().
		Set("filename", self.Filename).
		Set("attribute", self.Attributes)
}

func (self *XAttrFunction) getAttributeValues(scope vfilter.Scope, Attributes []string) {
	for _, attr := range Attributes {
		value, err := xattr.Get(self.Filename, attr)
		if err != nil {
			scope.Log("xattr: Failled to get attribute %s from filename %s: %s", attr, self.Filename, err)
			continue
		}
		self.Attributes[attr] = string(value)
	}
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
