// +build windows

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

// VQL plugins handy for registry parsing.
package windows

import (
	"context"
	"os"

	"github.com/Velocidex/ordereddict"
	"golang.org/x/sys/windows/registry"
	"www.velocidex.com/golang/velociraptor/acls"
	vql_subsystem "www.velocidex.com/golang/velociraptor/vql"
	vfilter "www.velocidex.com/golang/vfilter"
)

type _ExpandPathArgs struct {
	Path string `vfilter:"required,field=path,doc=A path with environment escapes"`
}

type _ExpandPath struct{}

func (self _ExpandPath) Call(
	ctx context.Context,
	scope vfilter.Scope,
	args *ordereddict.Dict) vfilter.Any {

	err := vql_subsystem.CheckAccess(scope, acls.MACHINE_STATE)
	if err != nil {
		scope.Log("expand: %s", err)
		return vfilter.Null{}
	}

	arg := &_ExpandPathArgs{}
	err = vfilter.ExtractArgs(scope, args, arg)
	if err != nil {
		scope.Log("expand: %s", err.Error())
		return vfilter.Null{}
	}

	// Support both go style expandsions and windows style
	// expansions.
	path := os.ExpandEnv(arg.Path)
	expanded_path, err := registry.ExpandString(path)
	if err != nil {
		scope.Log("expand: %v", err)
		return vfilter.Null{}
	}

	return expanded_path
}

func (self _ExpandPath) Info(scope vfilter.Scope, type_map *vfilter.TypeMap) *vfilter.FunctionInfo {
	return &vfilter.FunctionInfo{
		Name:    "expand",
		Doc:     "Expand the path using the environment.",
		ArgType: type_map.AddType(scope, &_ExpandPathArgs{}),
	}
}

func init() {
	vql_subsystem.RegisterFunction(&_ExpandPath{})
}
