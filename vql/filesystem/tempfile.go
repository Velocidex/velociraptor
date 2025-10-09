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
	"runtime"

	"github.com/Velocidex/ordereddict"
	"www.velocidex.com/golang/velociraptor/acls"
	utils_tempfile "www.velocidex.com/golang/velociraptor/utils/tempfile"
	"www.velocidex.com/golang/velociraptor/vql"
	vql_subsystem "www.velocidex.com/golang/velociraptor/vql"
	"www.velocidex.com/golang/vfilter"
	"www.velocidex.com/golang/vfilter/arg_parser"
)

type _TempfileRequest struct {
	Data        []string `vfilter:"optional,field=data,doc=Data to write in the tempfile."`
	Extension   string   `vfilter:"optional,field=extension,doc=An extension to place in the tempfile."`
	Permissions string   `vfilter:"optional,field=permissions,doc=Required permissions (e.g. 'x')."`
	RemoveLast  bool     `vfilter:"optional,field=remove_last,doc=If set we delay removal as much as possible."`
}

type TempfileFunction struct{}

func (self *TempfileFunction) Call(ctx context.Context,
	scope vfilter.Scope,
	args *ordereddict.Dict) vfilter.Any {

	defer vql_subsystem.RegisterMonitor(ctx, "tempfile", args)()

	err := vql_subsystem.CheckAccess(scope, acls.FILESYSTEM_WRITE)
	if err != nil {
		scope.Log("tempfile: %s", err)
		return false
	}

	arg := &_TempfileRequest{}
	err = arg_parser.ExtractArgsWithContext(ctx, scope, args, arg)
	if err != nil {
		scope.Log("tempfile: %s", err.Error())
		return false
	}

	permissions := os.FileMode(0600)
	switch arg.Permissions {
	case "x":
		permissions = 0700

		// On windows executable means it has a .exe extension.
		if runtime.GOOS == "windows" {
			arg.Extension = ".exe"
		}

	case "r":
		permissions = 0400
	}

	tmpfile, err := utils_tempfile.TempFile("tmp*" + arg.Extension)
	if err != nil {
		scope.Log("tempfile: %v", err)
		return false
	}

	utils_tempfile.AddTmpFile(tmpfile.Name())

	// Try to set the permissions to the desired level.
	_ = os.Chmod(tmpfile.Name(), permissions)

	for _, content := range arg.Data {
		_, err := tmpfile.Write([]byte(content))
		if err != nil {
			scope.Log("tempfile: %s", err.Error())
		}
	}

	if err := tmpfile.Close(); err != nil {
		scope.Log("tempfile: %s", err.Error())
		return &vfilter.Null{}
	}

	if arg.RemoveLast {
		scope.Log("tempfile: Adding global destructor for %v", tmpfile.Name())
		root_scope := vql_subsystem.GetRootScope(scope)
		err := root_scope.AddDestructor(func() {
			RemoveTmpFile(0, tmpfile.Name(), root_scope)
		})
		if err != nil {
			RemoveTmpFile(0, tmpfile.Name(), scope)
			scope.Log("tempfile: %v", err)
		}
	} else {
		err := scope.AddDestructor(func() {
			RemoveTmpFile(0, tmpfile.Name(), scope)
		})
		if err != nil {
			RemoveTmpFile(0, tmpfile.Name(), scope)
			scope.Log("tempfile: %v", err)
		}
	}
	return tmpfile.Name()
}

func (self TempfileFunction) Info(scope vfilter.Scope,
	type_map *vfilter.TypeMap) *vfilter.FunctionInfo {
	return &vfilter.FunctionInfo{
		Name:     "tempfile",
		Doc:      "Create a temporary file and write some data into it.",
		ArgType:  type_map.AddType(scope, &_TempfileRequest{}),
		Metadata: vql.VQLMetadata().Permissions(acls.FILESYSTEM_WRITE).Build(),
	}
}

type _TempdirRequest struct {
	RemoveLast bool `vfilter:"optional,field=remove_last,doc=If set we delay removal as much as possible."`
}

type TempdirFunction struct{}

func (self *TempdirFunction) Call(ctx context.Context,
	scope vfilter.Scope,
	args *ordereddict.Dict) vfilter.Any {

	defer vql_subsystem.RegisterMonitor(ctx, "tempdir", args)()

	err := vql_subsystem.CheckAccess(scope, acls.FILESYSTEM_WRITE)
	if err != nil {
		scope.Log("tempdir: %s", err)
		return false
	}

	arg := &_TempdirRequest{}
	err = arg_parser.ExtractArgsWithContext(ctx, scope, args, arg)
	if err != nil {
		scope.Log("tempdir: %s", err.Error())
		return false
	}

	dir, err := utils_tempfile.TempDir("tmp")
	if err != nil {
		scope.Log("tempdir: %v", err)
		return false
	}

	if arg.RemoveLast {
		scope.Log("tempdir: Adding global destructor for %v", dir)
		root_scope := vql_subsystem.GetRootScope(scope)
		err := root_scope.AddDestructor(func() {
			RemoveDirectory(0, dir, root_scope)
		})
		if err != nil {
			RemoveDirectory(0, dir, scope)
			scope.Log("tempdir: %v", err)
		}

	} else {
		err := scope.AddDestructor(func() {
			RemoveDirectory(0, dir, scope)
		})
		if err != nil {
			RemoveDirectory(0, dir, scope)
			scope.Log("tempdir: %v", err)
		}
	}
	return dir
}

func (self TempdirFunction) Info(scope vfilter.Scope,
	type_map *vfilter.TypeMap) *vfilter.FunctionInfo {
	return &vfilter.FunctionInfo{
		Name:     "tempdir",
		Doc:      "Create a temporary directory. The directory will be removed when the query ends.",
		ArgType:  type_map.AddType(scope, &_TempdirRequest{}),
		Metadata: vql.VQLMetadata().Permissions(acls.FILESYSTEM_WRITE).Build(),
	}
}

// Make sure the file is removed when the query is done.
func RemoveDirectory(retry int, tmpdir string, scope vfilter.Scope) {
	if retry >= 10 {
		scope.Log("RemoveDirectory: Retry count exceeded - giving up")
		return
	}

	if retry > 0 {
		scope.Log("RemoveDirectory: removing tempdir %v (Try %v)",
			tmpdir, retry)
	} else {
		scope.Log("RemoveDirectory: removing tempdir %v", tmpdir)
	}

	// On windows especially we can not remove files that
	// are opened by something else, so we keep trying for
	// a while.
	err := os.RemoveAll(tmpdir)
	if err != nil {
		scope.Log("RemoveDirectory: Failed to remove %v: %v, reschedule", tmpdir, err)

		// Add another detructor to try again a bit later.
		err = scope.AddDestructor(func() {
			RemoveTmpFile(retry+1, tmpdir, scope)
		})
		if err != nil {
			return
		}

	} else {
		if retry > 0 {
			scope.Log("RemoveDirectory: removed tempdir %v (Try %v)",
				tmpdir, retry)
		} else {
			scope.Log("RemoveDirectory: removed tempdir %v", tmpdir)
		}
	}
}

// Make sure the file is removed when the query is done.
func RemoveTmpFile(retry int, tmpfile string, scope vfilter.Scope) {
	if retry >= 10 {
		scope.Log("tempfile: Retry count exceeded - giving up")
		return
	}

	if retry > 0 {
		scope.Log("tempfile: removing tempfile %v (Try %v)",
			tmpfile, retry)
	}

	// On windows especially we can not remove files that
	// are opened by something else, so we keep trying for
	// a while.
	err := os.Remove(tmpfile)
	if err != nil {
		scope.Log("tempfile: Failed to remove %v: %v, reschedule", tmpfile, err)

		// Add another detructor to try again a bit later.
		err = scope.AddDestructor(func() {
			RemoveTmpFile(retry+1, tmpfile, scope)
		})
		if err != nil {
			return
		}
	} else {
		if retry > 0 {
			scope.Log("tempfile: removed tempfile %v (Try %v)",
				tmpfile, retry)
		} else {
			scope.Log("tempfile: removed tempfile %v", tmpfile)
		}
	}

	// Remove the file from the tracker.
	utils_tempfile.RemoveTmpFile(tmpfile, err)
}

func init() {
	vql_subsystem.RegisterFunction(&TempdirFunction{})
	vql_subsystem.RegisterFunction(&TempfileFunction{})
}
