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
/*

Searches the client index.

The client index is essentially a set membership database. Each index
term is a key, and each key contains a list of values within it. For
example:

key:   host:win1_us
value: ["C.1234"]
key:   host:win2_us
value: ["C.2345"]
key:   label:foobar
value: ["C.1234", "C.2345"]

For example, the key "label:foobar" contains all the clients with that
label attached.

If a query contains wild card characters (* or ?) then we return a
union of all the values with keys that matches the wildcards. For
example a query might be "host:win*_us" so we return a union of values
stored under the key "host:win1_us" and "host:win2_us" which might be
"C.1234" and "C.2345".

By default an unspecified query type returns the values stored under
the keys matching the query. If the query type is "key" then the query
will return all keys that match without fetching their values. For
example a query of "host:win*_us" with a query type of "key" will
return "host:win1_us" and "host:win2_us".

*/

package server

import (
	"context"

	api_proto "www.velocidex.com/golang/velociraptor/api/proto"
	"www.velocidex.com/golang/velociraptor/constants"
	"www.velocidex.com/golang/velociraptor/datastore"
	vql_subsystem "www.velocidex.com/golang/velociraptor/vql"
	"www.velocidex.com/golang/vfilter"
)

type SearchPluginArgs struct {
	// This searches for all search terms by approximate match.
	Query  string `vfilter:"optional,field=query"`
	Offset uint64 `vfilter:"optional,field=offset"`
	Limit  uint64 `vfilter:"optional,field=limit"`

	// If this is "key" then we return keys that match.
	Type string `vfilter:"optional,field=type"`
}

type SearchPlugin struct{}

func (self SearchPlugin) Call(
	ctx context.Context,
	scope *vfilter.Scope,
	args *vfilter.Dict) <-chan vfilter.Row {
	output_chan := make(chan vfilter.Row)

	go func() {
		defer close(output_chan)

		arg := &SearchPluginArgs{}
		err := vfilter.ExtractArgs(scope, args, arg)
		if err != nil {
			scope.Log("search: %v", err)
			return
		}

		if arg.Limit == 0 {
			arg.Limit = 10000
		}

		any_config_obj, _ := scope.Resolve("server_config")
		config_obj, ok := any_config_obj.(*api_proto.Config)
		if !ok {
			scope.Log("Command can only run on the server")
			return
		}

		db, err := datastore.GetDB(config_obj)
		if err != nil {
			return
		}

		for _, item := range db.SearchClients(
			config_obj, constants.CLIENT_INDEX_URN,
			arg.Query, arg.Type, arg.Offset, arg.Limit) {

			output_chan <- vfilter.NewDict().Set("Hit", item)
		}
	}()

	return output_chan
}

func (self SearchPlugin) Info(scope *vfilter.Scope,
	type_map *vfilter.TypeMap) *vfilter.PluginInfo {
	return &vfilter.PluginInfo{
		Name:    "search",
		Doc:     "Search the server client's index.",
		ArgType: type_map.AddType(scope, &SearchPluginArgs{}),
	}
}

func init() {
	vql_subsystem.RegisterPlugin(&SearchPlugin{})
}
