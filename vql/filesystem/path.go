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
	"regexp"

	vql_subsystem "www.velocidex.com/golang/velociraptor/vql"
	"www.velocidex.com/golang/vfilter"
)

var basename_regexp = regexp.MustCompile(`([^/\\]+)$`)

type _BasenameArgs struct {
	Path string `vfilter:"required,field=path,doc=The path to use"`
}

type _Basename struct{}

func (self _Basename) Call(
	ctx context.Context,
	scope *vfilter.Scope,
	args *vfilter.Dict) vfilter.Any {
	arg := &_BasenameArgs{}
	err := vfilter.ExtractArgs(scope, args, arg)
	if err != nil {
		scope.Log("basename: %s", err.Error())
		return vfilter.Null{}
	}

	match := basename_regexp.FindStringSubmatch(arg.Path)
	if match != nil {
		return match[1]
	}
	return "/"
}

func (self _Basename) Info(scope *vfilter.Scope, type_map *vfilter.TypeMap) *vfilter.FunctionInfo {
	return &vfilter.FunctionInfo{
		Name:    "basename",
		Doc:     "Splits the path on separator and return the basename.",
		ArgType: type_map.AddType(scope, &_BasenameArgs{}),
	}
}

func init() {
	vql_subsystem.RegisterFunction(&_Basename{})
}
