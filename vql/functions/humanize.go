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
package functions

import (
	"context"
	"fmt"

	humanize "github.com/dustin/go-humanize"
	vql_subsystem "www.velocidex.com/golang/velociraptor/vql"
	"www.velocidex.com/golang/vfilter"
)

type HumanizeArgs struct {
	Bytes int64 `vfilter:"optional,field=bytes"`
}

type HumanizeFunction struct{}

func (self *HumanizeFunction) Call(ctx context.Context,
	scope *vfilter.Scope,
	args *vfilter.Dict) vfilter.Any {
	arg := &HumanizeArgs{}
	err := vfilter.ExtractArgs(scope, args, arg)
	if err != nil {
		scope.Log("Humanize: %s", err.Error())
		return false
	}

	if arg.Bytes > 0 {
		return humanize.Bytes(uint64(arg.Bytes))
	}

	return fmt.Sprintf("%v", arg.Bytes)
}

func (self HumanizeFunction) Info(scope *vfilter.Scope, type_map *vfilter.TypeMap) *vfilter.FunctionInfo {
	return &vfilter.FunctionInfo{
		Name:    "humanize",
		Doc:     "Format items in human readable way.",
		ArgType: type_map.AddType(scope, &HumanizeArgs{}),
	}
}

func init() {
	vql_subsystem.RegisterFunction(&HumanizeFunction{})
}
