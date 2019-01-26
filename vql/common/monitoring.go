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
package common

import (
	"context"

	"www.velocidex.com/golang/vfilter"
)

type WatchPluginArgs struct {
	Period int64               `vfilter:"required,field=period"`
	Query  vfilter.StoredQuery `vfilter:"required,field=query"`
}

type WatchPlugin struct{}

func (self WatchPlugin) Call(
	ctx context.Context,
	scope *vfilter.Scope,
	args *vfilter.Dict) <-chan vfilter.Row {
	output_chan := make(chan vfilter.Row)

	go func() {
		defer close(output_chan)

		arg := &WatchPluginArgs{}
		err := vfilter.ExtractArgs(scope, args, arg)
		if err != nil {
			scope.Log("watch: %v", err)
			return
		}
	}()

	return output_chan
}

func (self WatchPlugin) Info(scope *vfilter.Scope, type_map *vfilter.TypeMap) *vfilter.PluginInfo {
	return &vfilter.PluginInfo{
		Name:    "watch",
		Doc:     "Run query periodically and watch for changes in output.",
		ArgType: type_map.AddType(scope, &WatchPluginArgs{}),
	}
}

func init() {
	// Not implemented yet.
	// vql_subsystem.RegisterPlugin(&WatchPlugin{})
}
