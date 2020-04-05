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
package parsers

import (
	"context"
	"io"

	"github.com/Velocidex/ordereddict"
	pe "www.velocidex.com/golang/go-pe"
	"www.velocidex.com/golang/velociraptor/glob"
	vql_subsystem "www.velocidex.com/golang/velociraptor/vql"
	vfilter "www.velocidex.com/golang/vfilter"
)

type _PEFunctionArgs struct {
	Filename string `vfilter:"required,field=file,doc=The PE file to open."`
	Accessor string `vfilter:"optional,field=accessor,doc=The accessor to use."`
}

type _PEFunction struct{}

func (self _PEFunction) Info(scope *vfilter.Scope, type_map *vfilter.TypeMap) *vfilter.FunctionInfo {
	return &vfilter.FunctionInfo{
		Name:    "parse_pe",
		Doc:     "Parse a PE file.",
		ArgType: type_map.AddType(scope, &_PEFunctionArgs{}),
	}
}

func (self _PEFunction) Call(
	ctx context.Context, scope *vfilter.Scope,
	args *ordereddict.Dict) vfilter.Any {
	arg := &_PEFunctionArgs{}
	err := vfilter.ExtractArgs(scope, args, arg)
	if err != nil {
		scope.Log("parse_pe: %v", err)
		return &vfilter.Null{}
	}

	err = vql_subsystem.CheckFilesystemAccess(scope, arg.Accessor)
	if err != nil {
		scope.Log("parse_pe: %s", err)
		return &vfilter.Null{}
	}

	accessor, err := glob.GetAccessor(arg.Accessor, ctx)
	if err != nil {
		scope.Log("parse_pe: %v", err)
		return &vfilter.Null{}
	}
	fd, err := accessor.Open(arg.Filename)
	if err != nil {
		scope.Log("parse_pe: %v", err)
		return &vfilter.Null{}
	}
	defer fd.Close()

	reader, ok := fd.(io.ReaderAt)
	if !ok {
		scope.Log("parse_pe: file is not seekable %s", arg.Filename)
		return &vfilter.Null{}
	}

	pe_file, err := pe.NewPEFile(reader)
	if err != nil {
		scope.Log("parse_pe: %v", err)
		return &vfilter.Null{}
	}

	return pe_file
}

func init() {
	vql_subsystem.RegisterFunction(&_PEFunction{})
}
