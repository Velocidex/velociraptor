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
	"os"
	"time"

	"github.com/Velocidex/ordereddict"
	"github.com/tink-ab/tempfile"
	vql_subsystem "www.velocidex.com/golang/velociraptor/vql"
	"www.velocidex.com/golang/vfilter"
)

type _TempfileRequest struct {
	Data      []string `vfilter:"optional,field=data,doc=Data to write in the tempfile."`
	Extension string   `vfilter:"optional,field=extension,doc=An extension to place in the tempfile."`
}

type TempfileFunction struct{}

func (self *TempfileFunction) Call(ctx context.Context,
	scope *vfilter.Scope,
	args *ordereddict.Dict) vfilter.Any {
	arg := &_TempfileRequest{}
	err := vfilter.ExtractArgs(scope, args, arg)
	if err != nil {
		scope.Log("tempfile: %s", err.Error())
		return false
	}

	tmpfile, err := tempfile.TempFile("", "tmp", arg.Extension)
	if err != nil {
		scope.Log("tempfile: %v", err)
		return false
	}

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
	scope.AddDestructor(func() {
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
	})
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

func init() {
	vql_subsystem.RegisterFunction(&TempfileFunction{})
}
