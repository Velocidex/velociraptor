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
package parsers

import (
	"context"
	"io"

	"github.com/Velocidex/ordereddict"
	pe "www.velocidex.com/golang/go-pe"
	"www.velocidex.com/golang/velociraptor/accessors"
	"www.velocidex.com/golang/velociraptor/acls"
	"www.velocidex.com/golang/velociraptor/constants"
	utils "www.velocidex.com/golang/velociraptor/utils"
	"www.velocidex.com/golang/velociraptor/vql"
	vql_subsystem "www.velocidex.com/golang/velociraptor/vql"
	"www.velocidex.com/golang/velociraptor/vql/readers"
	vfilter "www.velocidex.com/golang/vfilter"
	"www.velocidex.com/golang/vfilter/arg_parser"
)

type _PEFunctionArgs struct {
	Filename   *accessors.OSPath `vfilter:"required,field=file,doc=The PE file to open."`
	Accessor   string            `vfilter:"optional,field=accessor,doc=The accessor to use."`
	BaseOffset int64             `vfilter:"optional,field=base_offset,doc=The offset in the file for the base address."`
}

type _PEFunction struct{}

func (self _PEFunction) Info(scope vfilter.Scope, type_map *vfilter.TypeMap) *vfilter.FunctionInfo {
	return &vfilter.FunctionInfo{
		Name:     "parse_pe",
		Doc:      "Parse a PE file.",
		ArgType:  type_map.AddType(scope, &_PEFunctionArgs{}),
		Metadata: vql.VQLMetadata().Permissions(acls.FILESYSTEM_READ).Build(),
	}
}

func (self _PEFunction) Call(
	ctx context.Context, scope vfilter.Scope,
	args *ordereddict.Dict) vfilter.Any {

	defer utils.RecoverVQL(scope)
	defer vql_subsystem.RegisterMonitor(ctx, "parse_pe", args)()

	arg := &_PEFunctionArgs{}
	err := arg_parser.ExtractArgsWithContext(ctx, scope, args, arg)
	if err != nil {
		scope.Log("parse_pe: %v", err)
		return &vfilter.Null{}
	}

	lru_size := vql_subsystem.GetIntFromRow(scope, scope, constants.BINARY_CACHE_SIZE)
	paged_reader, err := readers.NewAccessorReader(
		scope, arg.Accessor, arg.Filename, int(lru_size))
	if err != nil {
		return &vfilter.Null{}
	}
	defer paged_reader.Close()

	var reader io.ReaderAt = paged_reader
	var reader_size int64 = paged_reader.MaxSize()

	if arg.BaseOffset > 0 {
		reader = utils.NewOffsetReader(reader, arg.BaseOffset,
			arg.BaseOffset+reader_size)
	}

	pe_file, err := pe.NewPEFileWithSize(reader, reader_size)
	if err != nil {
		// Suppress logging for invalid PE files.
		// scope.Log("parse_pe: %v for %v", err, arg.Filename)
		return &vfilter.Null{}
	}

	// Set the max hash size if needed
	hash_max_size := vql_subsystem.GetIntFromRow(
		scope, scope, constants.HASH_MAX_SIZE)
	if hash_max_size > 0 {
		pe.SetHashSizeLimit(int64(hash_max_size))
	}

	// Return a lazy object.
	return ordereddict.NewDict().
		Set("FileHeader", pe_file.FileHeader).
		Set("GUIDAge", pe_file.GUIDAge).
		Set("PDB", pe_file.PDB).
		Set("Directories", func() vfilter.Any {
			return pe_file.GetDirectories()
		}).
		Set("Sections", pe_file.Sections).
		Set("Resources", pe_file.Resources()).
		Set("VersionInformation", func() vfilter.Any {
			return pe_file.VersionInformation()
		}).
		Set("Imports", func() vfilter.Any {
			return pe_file.Imports()
		}).
		Set("Exports", func() vfilter.Any {
			return pe_file.Exports()
		}).
		Set("ExportRVAs", func() vfilter.Any {
			return pe_file.ExportRVAs()
		}).
		Set("Forwards", func() vfilter.Any {
			return pe_file.Forwards()
		}).
		Set("ImpHash", func() vfilter.Any {
			return pe_file.ImpHash()
		}).
		Set("Authenticode", func() vfilter.Any {
			defer utils.RecoverVQL(scope)

			info, err := pe.ParseAuthenticode(pe_file)
			if err != nil {
				return vfilter.Null{}
			}

			return pe.PKCS7ToOrderedDict(info)
		}).
		Set("AuthenticodeHash", func() vfilter.Any {
			res, err := pe_file.CalcHashToDict(ctx)
			if err != nil {
				res = ordereddict.NewDict()
			}
			return res
		})
}

func init() {
	vql_subsystem.RegisterFunction(&_PEFunction{})
}
