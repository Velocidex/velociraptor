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

// This plugin provides support for parsing sqlite files. Because we
// use the actual library we must provide it with a file on
// disk. Since VQL may specify an arbitrary accessor, we can make a
// temp copy of the sqlite file in order to query it. The temp copy
// remains alive for the duration of the query, and we will cache it.

package parsers

import (
	"context"
	"os"
	"reflect"
	"strings"

	"github.com/Velocidex/ordereddict"
	"github.com/jmoiron/sqlx"
	_ "github.com/mattn/go-sqlite3"
	"github.com/tink-ab/tempfile"
	"www.velocidex.com/golang/velociraptor/glob"
	utils "www.velocidex.com/golang/velociraptor/utils"
	vql_subsystem "www.velocidex.com/golang/velociraptor/vql"
	vfilter "www.velocidex.com/golang/vfilter"
)

type _SQLiteArgs struct {
	Filename string      `vfilter:"required,field=file"`
	Accessor string      `vfilter:"optional,field=accessor,doc=The accessor to use."`
	Query    string      `vfilter:"required,field=query"`
	Args     vfilter.Any `vfilter:"optional,field=args"`
}

type _SQLitePlugin struct{}

func (self _SQLitePlugin) Call(
	ctx context.Context,
	scope *vfilter.Scope,
	args *ordereddict.Dict) <-chan vfilter.Row {
	output_chan := make(chan vfilter.Row)
	go func() {
		defer close(output_chan)
		defer utils.RecoverVQL(scope)

		arg := &_SQLiteArgs{}
		err := vfilter.ExtractArgs(scope, args, arg)
		if err != nil {
			scope.Log("sqlite: %v", err)
			return
		}

		if arg.Accessor == "" {
			arg.Accessor = "file"
		}

		err = vql_subsystem.CheckFilesystemAccess(scope, arg.Accessor)
		if err != nil {
			scope.Log("sqlite: %s", err)
			return
		}

		handle, err := self.GetHandle(ctx, arg, scope)
		if err != nil {
			scope.Log("sqlite: %v", err)
			return
		}

		query_parameters := []interface{}{}
		if arg.Args != nil {
			args_value := reflect.Indirect(reflect.ValueOf(arg.Args))
			if args_value.Type().Kind() != reflect.Slice {
				query_parameters = append(query_parameters, arg.Args)
			} else {
				for i := 0; i < args_value.Len(); i++ {
					query_parameters = append(query_parameters,
						args_value.Index(i).Interface())
				}
			}
		}

		rows, err := handle.Queryx(arg.Query, query_parameters...)
		if err != nil {
			scope.Log("sqlite: %v", err)
			return
		}
		defer rows.Close()

		columns, err := rows.Columns()
		if err != nil {
			scope.Log("sqlite: %v", err)
			return
		}

		for rows.Next() {
			row := ordereddict.NewDict()
			values, err := rows.SliceScan()
			if err != nil {
				scope.Log("sqlite: %v", err)
				return
			}

			for idx, item := range columns {
				var value interface{} = values[idx]
				bytes_value, ok := value.([]byte)
				if ok {
					value = string(bytes_value)
				}
				row.Set(item, value)
			}

			output_chan <- row
		}

	}()

	return output_chan
}

func VFSPathToFilesystemPath(path string) string {
	return strings.TrimPrefix(path, "\\")
}

func (self _SQLitePlugin) GetHandle(
	ctx context.Context,
	arg *_SQLiteArgs, scope *vfilter.Scope) (
	handle *sqlx.DB, err error) {
	filename := VFSPathToFilesystemPath(arg.Filename)

	key := "sqlite_" + filename + arg.Accessor
	handle, ok := vql_subsystem.CacheGet(
		scope, key).(*sqlx.DB)
	if !ok {
		if arg.Accessor == "file" {
			handle, err = sqlx.Connect("sqlite3", filename)
			if err != nil {
				// An error occured maybe the database
				// is locked, we try to copy it to
				// temp file and try again.
				if arg.Accessor != "data" {
					scope.Log("Unable to open sqlite file %v: %v",
						filename, err)
				} else {
					scope.Log("Unable to open sqlite file: %v", err)
				}
				if !strings.Contains(err.Error(), "locked") {
					return nil, err
				}
				filename, err = self._MakeTempfile(ctx, arg, filename, scope)
				if err != nil {
					scope.Log("Unable to create temp file: %v", err)
					return nil, err
				}
			}
		} else {
			filename, err = self._MakeTempfile(ctx, arg, filename, scope)
			if err != nil {
				return nil, err
			}
		}
		if handle == nil {
			handle, err = sqlx.Connect("sqlite3", filename)
			if err != nil {
				return nil, err
			}
		}

		vql_subsystem.CacheSet(scope, key, handle)
		scope.AddDestructor(func() {
			handle.Close()
		})
	}
	return handle, nil
}

func (self _SQLitePlugin) _MakeTempfile(
	ctx context.Context,
	arg *_SQLiteArgs, filename string,
	scope *vfilter.Scope) (
	string, error) {

	if arg.Accessor != "data" {
		scope.Log("Will try to copy to temp file: %v", filename)
	}

	tmpfile, err := tempfile.TempFile("", "tmp", ".sqlite")
	if err != nil {
		return "", err
	}
	defer tmpfile.Close()

	// Make sure the file is removed when the query is done.
	scope.AddDestructor(func() {
		scope.Log("sqlite: removing tempfile %v", tmpfile.Name())
		err = os.Remove(tmpfile.Name())
		if err != nil {
			scope.Log("Error removing file: %v", err)
		}
	})

	fs, err := glob.GetAccessor(arg.Accessor, scope)
	if err != nil {
		return "", err
	}

	file, err := fs.Open(filename)
	if err != nil {
		return "", err
	}
	defer file.Close()

	_, err = utils.Copy(ctx, tmpfile, file)
	if err != nil {
		return "", err
	}

	return tmpfile.Name(), nil
}

func (self _SQLitePlugin) Info(scope *vfilter.Scope, type_map *vfilter.TypeMap) *vfilter.PluginInfo {
	return &vfilter.PluginInfo{
		Name:    "sqlite",
		Doc:     "Opens an SQLite file and run a query against it.",
		ArgType: type_map.AddType(scope, &_SQLiteArgs{}),
	}
}

func init() {
	vql_subsystem.RegisterPlugin(&_SQLitePlugin{})
}
