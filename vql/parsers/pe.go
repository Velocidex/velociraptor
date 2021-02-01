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

	"github.com/Velocidex/ordereddict"
	pe "www.velocidex.com/golang/go-pe"
	vql_subsystem "www.velocidex.com/golang/velociraptor/vql"
	"www.velocidex.com/golang/velociraptor/vql/readers"
	vfilter "www.velocidex.com/golang/vfilter"
)

type _PEFunctionArgs struct {
	Filename string `vfilter:"required,field=file,doc=The PE file to open."`
	Accessor string `vfilter:"optional,field=accessor,doc=The accessor to use."`
}

type _PEFunction struct{}

func (self _PEFunction) Info(scope vfilter.Scope, type_map *vfilter.TypeMap) *vfilter.FunctionInfo {
	return &vfilter.FunctionInfo{
		Name:    "parse_pe",
		Doc:     "Parse a PE file.",
		ArgType: type_map.AddType(scope, &_PEFunctionArgs{}),
	}
}

func (self _PEFunction) Call(
	ctx context.Context, scope vfilter.Scope,
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

	paged_reader := readers.NewPagedReader(scope, arg.Accessor, arg.Filename)
	pe_file, err := pe.NewPEFile(paged_reader)
	if err != nil {
		scope.Log("parse_pe: %v for %v", err, arg.Filename)
		return &vfilter.Null{}
	}

	// Return a lazy object.
	return ordereddict.NewDict().
		Set("FileHeader", pe_file.FileHeader).
		Set("GUIDAge", pe_file.GUIDAge).
		Set("PDB", pe_file.PDB).
		Set("Sections", pe_file.Sections).
		Set("VersionInformation", func() vfilter.Any {
			return pe_file.VersionInformation()
		}).
		Set("Imports", func() vfilter.Any {
			return pe_file.Imports()
		}).
		Set("Exports", func() vfilter.Any {
			return pe_file.Exports()
		}).
		Set("Forwards", func() vfilter.Any {
			return pe_file.Forwards()
		}).
		Set("ImpHash", func() vfilter.Any {
			return pe_file.ImpHash()
		})
}

func init() {
	vql_subsystem.RegisterFunction(&_PEFunction{})
}
