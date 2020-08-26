/*
   Velociraptor - Hunting Evil
   Copyright (C) 2019 Velocidex Innovations.

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
	"io/ioutil"
	"os"
	"runtime"
	"time"

	"github.com/Velocidex/ordereddict"
	"www.velocidex.com/golang/velociraptor/acls"
	"www.velocidex.com/golang/velociraptor/constants"
	vql_subsystem "www.velocidex.com/golang/velociraptor/vql"
	"www.velocidex.com/golang/vfilter"
)

type _TempfileRequest struct {
	Data        []string `vfilter:"optional,field=data,doc=Data to write in the tempfile."`
	Extension   string   `vfilter:"optional,field=extension,doc=An extension to place in the tempfile."`
	Permissions string   `vfilter:"optional,field=permissions,doc=Required permissions (e.g. 'x')."`
	RemoveLast  bool     `vfilter:"optional,field=remove_last,doc=If set we delay removal as much as possible."`
}

type TempfileFunction struct{}

func (self *TempfileFunction) Call(ctx context.Context,
	scope *vfilter.Scope,
	args *ordereddict.Dict) vfilter.Any {

	err := vql_subsystem.CheckAccess(scope, acls.FILESYSTEM_WRITE)
	if err != nil {
		scope.Log("tempfile: %s", err)
		return false
	}

	arg := &_TempfileRequest{}
	err = vfilter.ExtractArgs(scope, args, arg)
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

	tmpfile, err := ioutil.TempFile("", "tmp*"+arg.Extension)
	if err != nil {
		scope.Log("tempfile: %v", err)
		return false
	}

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

	// Make sure the file is removed when the query is done.
	removal := func() {
		scope.Log("tempfile: removing tempfile %v", tmpfile.Name())

		// On windows especially we can not remove files that
		// are opened by something else, so we keep trying for
		// a while.
		for i := 0; i < 100; i++ {
			err := os.Remove(tmpfile.Name())
			if err == nil {
				break
			}
			time.Sleep(time.Second)
		}
	}

	if arg.RemoveLast {
		root_any, pres := scope.Resolve(constants.SCOPE_ROOT)
		if pres {
			root, ok := root_any.(*vfilter.Scope)
			if ok {
				scope.Log("Adding global destructor for %v", tmpfile.Name())
				root.AddDestructor(removal)
			}
		}
	} else {
		scope.AddDestructor(removal)
	}
	return tmpfile.Name()
}

func (self TempfileFunction) Info(scope *vfilter.Scope,
	type_map *vfilter.TypeMap) *vfilter.FunctionInfo {
	return &vfilter.FunctionInfo{
		Name:    "tempfile",
		Doc:     "Create a temporary file and write some data into it.",
		ArgType: type_map.AddType(scope, &_TempfileRequest{}),
	}
}

type _TempdirRequest struct {
	RemoveLast bool `vfilter:"optional,field=remove_last,doc=If set we delay removal as much as possible."`
}

type TempdirFunction struct{}

func (self *TempdirFunction) Call(ctx context.Context,
	scope *vfilter.Scope,
	args *ordereddict.Dict) vfilter.Any {

	err := vql_subsystem.CheckAccess(scope, acls.FILESYSTEM_WRITE)
	if err != nil {
		scope.Log("tempdir: %s", err)
		return false
	}

	arg := &_TempdirRequest{}
	err = vfilter.ExtractArgs(scope, args, arg)
	if err != nil {
		scope.Log("tempdir: %s", err.Error())
		return false
	}

	dir, err := ioutil.TempDir("", "tmp")
	if err != nil {
		scope.Log("tempdir: %v", err)
		return false
	}

	// Make sure the file is removed when the query is done.
	removal := func() {
		scope.Log("tempfile: removing tempfile %v", dir)

		// On windows especially we can not remove files that
		// are opened by something else, so we keep trying for
		// a while.
		for i := 0; i < 100; i++ {
			err := os.RemoveAll(dir)
			if err == nil {
				break
			}
			time.Sleep(time.Second)
		}
	}

	if arg.RemoveLast {
		root_any, pres := scope.Resolve(constants.SCOPE_ROOT)
		if pres {
			root, ok := root_any.(*vfilter.Scope)
			if ok {
				scope.Log("Adding global destructor for %v", dir)
				root.AddDestructor(removal)
			}
		}
	} else {
		scope.AddDestructor(removal)
	}
	return dir
}

func (self TempdirFunction) Info(scope *vfilter.Scope,
	type_map *vfilter.TypeMap) *vfilter.FunctionInfo {
	return &vfilter.FunctionInfo{
		Name:    "tempdir",
		Doc:     "Create a temporary directory. The directory will be removed when the query ends.",
		ArgType: type_map.AddType(scope, &_TempdirRequest{}),
	}
}

func init() {
	vql_subsystem.RegisterFunction(&TempdirFunction{})
	vql_subsystem.RegisterFunction(&TempfileFunction{})
}
