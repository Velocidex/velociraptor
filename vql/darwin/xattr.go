//go:build linux || darwin

package darwin

import (
	"context"

	"golang.org/x/sys/unix"

	"github.com/Velocidex/ordereddict"
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

	defer vql_subsystem.RegisterMonitor(ctx, "xattr", args)()
	defer vql_subsystem.CheckForPanic(scope, "xattr")

	arg := &XAttrArgs{}
	err := arg_parser.ExtractArgsWithContext(ctx, scope, args, arg)
	if err != nil {
		scope.Log("xattr: Arg parser: %s", err)
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
		attributes, err := List(filename)
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
		value, err := Get(Filename, attr)
		if err != nil {
			continue
		}
		ret.Set(attr, string(value))
	}
	return ret
}

func (self XAttrFunction) Info(
	scope vfilter.Scope, type_map *vfilter.TypeMap) *vfilter.FunctionInfo {
	return &vfilter.FunctionInfo{
		Name:     "xattr",
		Doc:      "Query a file for the specified extended attribute.",
		ArgType:  type_map.AddType(scope, &XAttrArgs{}),
		Metadata: vql_subsystem.VQLMetadata().Permissions(acls.FILESYSTEM_READ).Build(),
	}
}

func init() {
	vql_subsystem.RegisterFunction(&XAttrFunction{})
}

// Retrieves extended attribute data associated with path.
func Get(path, attr string) ([]byte, error) {
	attr = prefix + attr

	// find size
	size, err := unix.Getxattr(path, attr, nil)
	if err != nil {
		return nil, err
	}

	if size <= 0 {
		return nil, err
	}

	buf := make([]byte, size)
	size, err = unix.Getxattr(path, attr, buf)
	if err != nil {
		return nil, err
	}
	return buf[:size], nil
}

// Retrieves a list of names of extended attributes associated with path.
func List(path string) ([]string, error) {
	// find size
	size, err := unix.Listxattr(path, nil)
	if err != nil {
		return nil, err
	}
	if size == 0 {
		return []string{}, nil
	}

	// read into buffer of that size
	buf := make([]byte, size)
	size, err = unix.Listxattr(path, buf)
	if err != nil {
		return nil, err
	}
	return stripPrefix(nullTermToStrings(buf[:size])), nil
}

// Associates data as an extended attribute of path.
func Set(path, attr string, data []byte) error {
	attr = prefix + attr
	return unix.Setxattr(path, attr, data, 0)
}

// Converts an array of NUL terminated UTF-8 strings
// to a []string.
func nullTermToStrings(buf []byte) (result []string) {
	offset := 0
	for index, b := range buf {
		if b == 0 {
			result = append(result, string(buf[offset:index]))
			offset = index + 1
		}
	}
	return
}
