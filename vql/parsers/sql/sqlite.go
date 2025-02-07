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

// This plugin provides support for parsing sqlite files. Because we
// use the actual library we must provide it with a file on
// disk. Since VQL may specify an arbitrary accessor, we can make a
// temp copy of the sqlite file in order to query it. The temp copy
// remains alive for the duration of the query, and we will cache it.
package sql

import (
	"context"

	"github.com/Velocidex/ordereddict"
	_ "github.com/mattn/go-sqlite3"
	"www.velocidex.com/golang/velociraptor/accessors"
	vql_subsystem "www.velocidex.com/golang/velociraptor/vql"
	vfilter "www.velocidex.com/golang/vfilter"
)

type SQLiteArgs struct {
	Filename *accessors.OSPath `vfilter:"required,field=file"`
	Accessor string            `vfilter:"optional,field=accessor,doc=The accessor to use."`
	Query    string            `vfilter:"required,field=query"`
	Args     vfilter.Any       `vfilter:"optional,field=args"`
}

type SQLitePlugin struct{}

func (self SQLitePlugin) Call(
	ctx context.Context,
	scope vfilter.Scope,
	args *ordereddict.Dict) <-chan vfilter.Row {

	args.Set("driver", "sqlite")

	// This is just an alias for the sql plugin.
	return SQLPlugin{}.Call(ctx, scope, args)
}

func (self SQLitePlugin) Info(scope vfilter.Scope, type_map *vfilter.TypeMap) *vfilter.PluginInfo {
	return &vfilter.PluginInfo{
		Name:    "sqlite",
		Doc:     "Opens an SQLite file and run a query against it (This is an alias to the sql() plugin which supports more database types).",
		ArgType: type_map.AddType(scope, &SQLiteArgs{}),
	}
}

func init() {
	vql_subsystem.RegisterPlugin(&SQLitePlugin{})
}
