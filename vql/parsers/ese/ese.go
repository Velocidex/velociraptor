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

package ese

import (
	"context"
	"encoding/hex"
	"errors"

	"github.com/Velocidex/ordereddict"
	"www.velocidex.com/golang/go-ese/parser"
	ntfs "www.velocidex.com/golang/go-ntfs/parser"
	"www.velocidex.com/golang/velociraptor/glob"
	utils "www.velocidex.com/golang/velociraptor/utils"
	vql_subsystem "www.velocidex.com/golang/velociraptor/vql"
	vfilter "www.velocidex.com/golang/vfilter"
	"www.velocidex.com/golang/vfilter/arg_parser"
)

type SRUMId struct {
	IdType  int64  `vfilter:"required,field=IdType"`
	IdIndex int64  `vfilter:"required,field=IdIndex"`
	IdBlob  string `vfilter:"optional,field=IdBlob"`
}

type _SRUMLookupIdArgs struct {
	Filename string `vfilter:"required,field=file"`
	Accessor string `vfilter:"optional,field=accessor,doc=The accessor to use."`
	Id       int64  `vfilter:"required,field=id"`
}

type _SRUMLookupId struct{}

func (self _SRUMLookupId) Info(scope vfilter.Scope, type_map *vfilter.TypeMap) *vfilter.FunctionInfo {
	return &vfilter.FunctionInfo{
		Name:    "srum_lookup_id",
		Doc:     "Lookup a SRUM id.",
		ArgType: type_map.AddType(scope, &_SRUMLookupIdArgs{}),
	}
}

func (self _SRUMLookupId) Call(
	ctx context.Context, scope vfilter.Scope,
	args *ordereddict.Dict) vfilter.Any {

	defer utils.RecoverVQL(scope)

	arg := &_SRUMLookupIdArgs{}
	err := arg_parser.ExtractArgsWithContext(ctx, scope, args, arg)
	if err != nil {
		scope.Log("srum_lookup_id: %v", err)
		return &vfilter.Null{}
	}

	err = vql_subsystem.CheckFilesystemAccess(scope, arg.Accessor)
	if err != nil {
		scope.Log("srum_lookup_id: %s", err)
		return &vfilter.Null{}
	}

	key := arg.Filename + arg.Accessor
	lookup_map, ok := vql_subsystem.CacheGet(scope, key).(map[int64]string)
	if !ok {
		lookup_map = make(map[int64]string)
		defer vql_subsystem.CacheSet(scope, key, lookup_map)

		accessor, err := glob.GetAccessor(arg.Accessor, scope)
		if err != nil {
			scope.Log("srum_lookup_id: %v", err)
			return &vfilter.Null{}
		}
		fd, err := accessor.Open(arg.Filename)
		if err != nil {
			scope.Log("parse_ese: Unable to open file %s: %v",
				arg.Filename, err)
			return &vfilter.Null{}
		}
		defer fd.Close()

		reader, err := ntfs.NewPagedReader(
			utils.ReaderAtter{Reader: fd}, 1024, 10000)
		if err != nil {
			scope.Log("parse_mft: Unable to open file %s: %v",
				arg.Filename, err)
			return &vfilter.Null{}
		}

		ese_ctx, err := parser.NewESEContext(reader)
		if err != nil {
			scope.Log("parse_ese: Unable to open file %s: %v",
				arg.Filename, err)
			return &vfilter.Null{}
		}

		catalog, err := parser.ReadCatalog(ese_ctx)
		if err != nil {
			scope.Log("parse_ese: Unable to open file %s: %v",
				arg.Filename, err)
			return &vfilter.Null{}
		}

		scope.Log("Parsing SruDbIdMapTable for %v", arg.Filename)
		err = catalog.DumpTable("SruDbIdMapTable", func(row *ordereddict.Dict) error {
			id_details := &SRUMId{}
			err := arg_parser.ExtractArgsWithContext(ctx, scope, row, id_details)
			if err != nil {
				return err
			}

			// Its a GUID
			if id_details.IdType == 3 {
				id_details.IdBlob = formatGUI(id_details.IdBlob)
			} else {
				id_details.IdBlob = formatString(id_details.IdBlob)
			}

			lookup_map[id_details.IdIndex] = id_details.IdBlob
			return nil
		})
		if err != nil {
			scope.Log("parse_ese: Unable to open file %s: %v",
				arg.Filename, err)
			return &vfilter.Null{}
		}
		scope.Log("Parsed %v successfully with %v records", arg.Filename,
			len(lookup_map))
	}

	value, pres := lookup_map[arg.Id]
	if !pres {
		return vfilter.Null{}
	}

	return value
}

func formatString(hexencoded string) string {
	buffer, err := hex.DecodeString(hexencoded)
	if err != nil {
		return hexencoded
	}

	return ParseTerminatedUTF16String(&utils.BufferReaderAt{Buffer: buffer}, 0)
}

func formatGUI(hexencoded string) string {
	if len(hexencoded) == 0 {
		return hexencoded
	}

	buffer, err := hex.DecodeString(hexencoded)
	if err != nil {
		return hexencoded
	}

	profile := NewMiscProfile()
	return profile.SID(&utils.BufferReaderAt{Buffer: buffer}, 0).String()
}

type _ESEArgs struct {
	Filename string `vfilter:"required,field=file"`
	Accessor string `vfilter:"optional,field=accessor,doc=The accessor to use."`
	Table    string `vfilter:"required,field=table,doc=A table name to dump"`
}

type _ESEPlugin struct{}

func (self _ESEPlugin) Call(
	ctx context.Context,
	scope vfilter.Scope,
	args *ordereddict.Dict) <-chan vfilter.Row {
	output_chan := make(chan vfilter.Row)
	go func() {
		defer close(output_chan)
		defer utils.RecoverVQL(scope)

		arg := &_ESEArgs{}
		err := arg_parser.ExtractArgsWithContext(ctx, scope, args, arg)
		if err != nil {
			scope.Log("parse_ese: %v", err)
			return
		}

		if arg.Accessor == "" {
			arg.Accessor = "file"
		}

		err = vql_subsystem.CheckFilesystemAccess(scope, arg.Accessor)
		if err != nil {
			scope.Log("parse_ese: %s", err)
			return
		}

		accessor, err := glob.GetAccessor(arg.Accessor, scope)
		if err != nil {
			scope.Log("parse_ese: %v", err)
			return
		}
		fd, err := accessor.Open(arg.Filename)
		if err != nil {
			scope.Log("parse_ese: Unable to open file %s: %v",
				arg.Filename, err)
			return
		}
		defer fd.Close()

		reader, err := ntfs.NewPagedReader(
			utils.ReaderAtter{Reader: fd}, 1024, 10000)
		if err != nil {
			scope.Log("parse_mft: Unable to open file %s: %v",
				arg.Filename, err)
			return
		}

		ese_ctx, err := parser.NewESEContext(reader)
		if err != nil {
			scope.Log("parse_ese: Unable to open file %s: %v",
				arg.Filename, err)
			return
		}

		catalog, err := parser.ReadCatalog(ese_ctx)
		if err != nil {
			scope.Log("parse_ese: Unable to open file %s: %v",
				arg.Filename, err)
			return
		}

		err = catalog.DumpTable(arg.Table, func(row *ordereddict.Dict) error {
			select {
			case <-ctx.Done():
				return errors.New("Query is cancelled")
			case output_chan <- row:
			}
			return nil
		})
		if err != nil {
			scope.Log("parse_ese: Unable to dump file %s: %v",
				arg.Filename, err)
			return
		}
	}()

	return output_chan
}

func (self _ESEPlugin) Info(scope vfilter.Scope, type_map *vfilter.TypeMap) *vfilter.PluginInfo {
	return &vfilter.PluginInfo{
		Name:    "parse_ese",
		Doc:     "Opens an ESE file and dump a table.",
		ArgType: type_map.AddType(scope, &_ESEArgs{}),
	}
}

func init() {
	vql_subsystem.RegisterPlugin(&_ESEPlugin{})
	vql_subsystem.RegisterFunction(&_SRUMLookupId{})
}
