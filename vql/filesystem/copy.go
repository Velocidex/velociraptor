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
package filesystem

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/Velocidex/ordereddict"
	"www.velocidex.com/golang/velociraptor/accessors"
	"www.velocidex.com/golang/velociraptor/accessors/file"
	"www.velocidex.com/golang/velociraptor/acls"
	"www.velocidex.com/golang/velociraptor/artifacts"
	"www.velocidex.com/golang/velociraptor/utils"
	"www.velocidex.com/golang/velociraptor/vql"
	vql_subsystem "www.velocidex.com/golang/velociraptor/vql"
	vfilter "www.velocidex.com/golang/vfilter"
	"www.velocidex.com/golang/vfilter/arg_parser"
)

type CopyFunctionArgs struct {
	Filename    *accessors.OSPath `vfilter:"required,field=filename,doc=The file to copy from."`
	Accessor    string            `vfilter:"optional,field=accessor,doc=The accessor to use"`
	Destination string            `vfilter:"required,field=dest,doc=The destination file to write."`
	Permissions string            `vfilter:"optional,field=permissions,doc=Required permissions (e.g. 'x')."`
	Append      bool              `vfilter:"optional,field=append,doc=If true we append to the target file otherwise truncate it"`
	Directories bool              `vfilter:"optional,field=create_directories,doc=If true we ensure the destination directories exist"`
}

type CopyFunction struct{}

func (self *CopyFunction) Call(ctx context.Context,
	scope vfilter.Scope,
	args *ordereddict.Dict) vfilter.Any {
	defer vql_subsystem.RegisterMonitor(ctx, "copy", args)()

	select {
	case <-ctx.Done():
		return vfilter.Null{}
	default:
	}

	// Check the config if we are allowed to execve at all.
	config_obj, ok := artifacts.GetConfig(scope)
	if ok && config_obj.PreventExecve {
		scope.Log("copy: Not allowed to write by configuration.")
		return vfilter.Null{}
	}

	arg := &CopyFunctionArgs{}
	err := arg_parser.ExtractArgsWithContext(ctx, scope, args, arg)
	if err != nil {
		scope.Log("copy: %v", err)
		return vfilter.Null{}
	}

	accessor, err := accessors.GetAccessor(arg.Accessor, scope)
	if err != nil {
		scope.Log("copy: %v", err)
		return vfilter.Null{}
	}

	fd, err := accessor.OpenWithOSPath(arg.Filename)
	if err != nil {
		scope.Log("copy: Failed to open %v: %v",
			arg.Filename, err)
		return vfilter.Null{}
	}
	defer fd.Close()

	permissions := os.FileMode(0600)

	switch arg.Permissions {
	case "x":
		permissions = 0700

		// On windows executable means it has a .exe extension.
		if runtime.GOOS == "windows" &&
			!strings.HasSuffix(arg.Destination, ".exe") {
			arg.Destination += ".exe"
		}

	case "r":
		permissions = 0400
	}

	// Report the command we ran for auditing
	// purposes. This will be collected in the flow logs.
	if arg.Accessor != "data" {
		scope.Log("copy: Copying file from %v into %v", arg.Filename,
			arg.Destination)
	}

	// We are about to write on the filesystem - make sure the user
	// has write access.
	err = vql_subsystem.CheckAccess(scope, acls.FILESYSTEM_WRITE)
	if err != nil {
		scope.Log("copy: %s", err.Error())
		return vfilter.Null{}
	}

	// Make sure we are allowed to write there.
	err = file.CheckPath(arg.Destination)
	if err != nil {
		scope.Log("copy: %s", err.Error())
		return vfilter.Null{}
	}

	flags := os.O_RDWR | os.O_CREATE | os.O_TRUNC
	if arg.Append {
		flags = os.O_WRONLY | os.O_APPEND
	}

	if arg.Directories {
		err = os.MkdirAll(filepath.Dir(arg.Destination), 0700)
		if err != nil {
			scope.Log("copy: Failed to create directories for %v: %v",
				arg.Destination, err)
			return vfilter.Null{}
		}
	}

	// Make sure the file is fully closed when the scope is destroyed.
	sub_ctx, cancel := context.WithCancel(ctx)
	_ = scope.AddDestructor(cancel)

	to, err := os.OpenFile(arg.Destination, flags, permissions)
	if err != nil {
		scope.Log("copy: Failed to open %v for writing: %v",
			arg.Destination, err)
		return vfilter.Null{}
	}
	defer to.Close()

	_, err = utils.Copy(sub_ctx, to, fd)
	if err != nil {
		scope.Log("copy: Failed to copy: %v", err)
		return vfilter.Null{}
	}

	return arg.Destination
}

func (self CopyFunction) Info(scope vfilter.Scope, type_map *vfilter.TypeMap) *vfilter.FunctionInfo {
	return &vfilter.FunctionInfo{
		Name:     "copy",
		Doc:      "Copy a file.",
		ArgType:  type_map.AddType(scope, &CopyFunctionArgs{}),
		Metadata: vql.VQLMetadata().Permissions(acls.FILESYSTEM_WRITE, acls.FILESYSTEM_READ).Build(),
	}
}

func init() {
	vql_subsystem.RegisterFunction(&CopyFunction{})
}
