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

package ese

import (
	"context"
	"encoding/hex"
	"errors"

	"github.com/Velocidex/ordereddict"
	"www.velocidex.com/golang/go-ese/parser"
	"www.velocidex.com/golang/velociraptor/accessors"
	"www.velocidex.com/golang/velociraptor/acls"
	utils "www.velocidex.com/golang/velociraptor/utils"
	"www.velocidex.com/golang/velociraptor/vql"
	vql_subsystem "www.velocidex.com/golang/velociraptor/vql"
	"www.velocidex.com/golang/velociraptor/vql/readers"
	vfilter "www.velocidex.com/golang/vfilter"
	"www.velocidex.com/golang/vfilter/arg_parser"
)

var (
	STOP_ERROR = errors.New("Stop")
)

type SRUMId struct {
	IdType  int64  `vfilter:"required,field=IdType"`
	IdIndex int64  `vfilter:"required,field=IdIndex"`
	IdBlob  string `vfilter:"optional,field=IdBlob"`
}

type _SRUMLookupIdArgs struct {
	Filename *accessors.OSPath `vfilter:"required,field=file"`
	Accessor string            `vfilter:"optional,field=accessor,doc=The accessor to use."`
	Id       int64             `vfilter:"required,field=id"`
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

	defer vql_subsystem.RegisterMonitor(ctx, "srum_lookup_id", args)()
	defer utils.RecoverVQL(scope)

	arg := &_SRUMLookupIdArgs{}
	err := arg_parser.ExtractArgsWithContext(ctx, scope, args, arg)
	if err != nil {
		scope.Log("srum_lookup_id: %v", err)
		return &vfilter.Null{}
	}

	key := arg.Filename.String() + arg.Accessor
	lookup_map, ok := vql_subsystem.CacheGet(scope, key).(map[int64]string)
	if !ok {
		lookup_map = make(map[int64]string)
		defer vql_subsystem.CacheSet(scope, key, lookup_map)

		// Use a managed reader
		reader, err := readers.NewAccessorReader(scope, arg.Accessor, arg.Filename, 10000)
		if err != nil {
			scope.Log("srum_lookup_id: Unable to open file %s: %v",
				arg.Filename, err)
			return &vfilter.Null{}
		}
		defer reader.Close()

		ese_ctx, err := parser.NewESEContext(reader)
		if err != nil {
			scope.Log("srum_lookup_id: Unable to open file %s: %v",
				arg.Filename, err)
			return &vfilter.Null{}
		}

		catalog, err := parser.ReadCatalog(ese_ctx)
		if err != nil {
			scope.Log("srum_lookup_id: Unable to open file %s: %v",
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
				id_details.IdBlob = formatGUID(id_details.IdBlob)
			} else {
				id_details.IdBlob = formatString(id_details.IdBlob)
			}

			lookup_map[id_details.IdIndex] = id_details.IdBlob
			return nil
		})
		if err != nil {
			scope.Log("srum_lookup_id: Unable to open file %s: %v",
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

func formatGUID(hexencoded string) string {
	if len(hexencoded) == 0 {
		return hexencoded
	}

	buffer, err := hex.DecodeString(hexencoded)
	if err != nil {
		return hexencoded
	}

	profile := NewMiscProfile()
	result := profile.SID(&utils.BufferReaderAt{Buffer: buffer}, 0)
	return result.String()
}

type _ESEArgs struct {
	Filename *accessors.OSPath `vfilter:"required,field=file"`
	Accessor string            `vfilter:"optional,field=accessor,doc=The accessor to use."`
	Table    string            `vfilter:"required,field=table,doc=A table name to dump"`
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
		defer vql_subsystem.RegisterMonitor(ctx, "parse_ese", args)()

		arg := &_ESEArgs{}
		err := arg_parser.ExtractArgsWithContext(ctx, scope, args, arg)
		if err != nil {
			scope.Log("parse_ese: %v", err)
			return
		}

		if arg.Accessor == "" {
			arg.Accessor = "auto"
		}

		reader, err := readers.NewAccessorReader(scope, arg.Accessor, arg.Filename, 10000)
		if err != nil {
			scope.Log("parse_ese: %v", err)
			return
		}
		defer reader.Close()

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
				return STOP_ERROR
			case output_chan <- row:
			}
			return nil
		})
		if err == nil || err == STOP_ERROR {
			return
		}

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
		Name:     "parse_ese",
		Doc:      "Opens an ESE file and dump a table.",
		ArgType:  type_map.AddType(scope, &_ESEArgs{}),
		Metadata: vql.VQLMetadata().Permissions(acls.FILESYSTEM_READ).Build(),
	}
}

type _ESECatalogArgs struct {
	Filename *accessors.OSPath `vfilter:"required,field=file"`
	Accessor string            `vfilter:"optional,field=accessor,doc=The accessor to use."`
}

type _ESECatalogPlugin struct{}

func (self _ESECatalogPlugin) Call(
	ctx context.Context,
	scope vfilter.Scope,
	args *ordereddict.Dict) <-chan vfilter.Row {
	output_chan := make(chan vfilter.Row)
	go func() {
		defer close(output_chan)
		defer utils.RecoverVQL(scope)
		defer vql_subsystem.RegisterMonitor(ctx, "parse_ese_catalog", args)()

		arg := &_ESECatalogArgs{}
		err := arg_parser.ExtractArgsWithContext(ctx, scope, args, arg)
		if err != nil {
			scope.Log("parse_ese_catalog: %v", err)
			return
		}

		if arg.Accessor == "" {
			arg.Accessor = "auto"
		}

		reader, err := readers.NewAccessorReader(scope, arg.Accessor, arg.Filename, 10000)
		if err != nil {
			scope.Log("parse_ese_catalog: %v", err)
			return
		}
		defer reader.Close()

		ese_ctx, err := parser.NewESEContext(reader)
		if err != nil {
			scope.Log("parse_ese_catalog: Unable to open file %s: %v",
				arg.Filename, err)
			return
		}

		catalog, err := parser.ReadCatalog(ese_ctx)
		if err != nil {
			scope.Log("parse_ese_catalog: Unable to open file %s: %v",
				arg.Filename, err)
			return
		}

		for _, i := range catalog.Tables.Items() {
			table := i.Value.(*parser.Table)

			for _, column := range table.Columns {
				select {
				case <-ctx.Done():
					return
				case output_chan <- ordereddict.NewDict().
					Set("Table", i.Key).
					Set("Column", column.Name).
					Set("Type", column.Type):
				}
			}
		}
	}()

	return output_chan
}

func (self _ESECatalogPlugin) Info(scope vfilter.Scope, type_map *vfilter.TypeMap) *vfilter.PluginInfo {
	return &vfilter.PluginInfo{
		Name:     "parse_ese_catalog",
		Doc:      "Opens an ESE file and dump the schema.",
		ArgType:  type_map.AddType(scope, &_ESECatalogArgs{}),
		Metadata: vql.VQLMetadata().Permissions(acls.FILESYSTEM_READ).Build(),
	}
}

func init() {
	vql_subsystem.RegisterPlugin(&_ESEPlugin{})
	vql_subsystem.RegisterPlugin(&_ESECatalogPlugin{})
	vql_subsystem.RegisterFunction(&_SRUMLookupId{})
}
