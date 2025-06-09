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
package functions

import (
	"context"
	"time"

	"github.com/Velocidex/ordereddict"
	humanize "github.com/dustin/go-humanize"
	vql_subsystem "www.velocidex.com/golang/velociraptor/vql"
	"www.velocidex.com/golang/vfilter"
	"www.velocidex.com/golang/vfilter/arg_parser"
)

type HumanizeArgs struct {
	Bytes  int64     `vfilter:"optional,field=bytes,doc=Format bytes with units (e.g. MB)"`
	IBytes int64     `vfilter:"optional,field=ibytes,doc=Format bytes with units (e.g. MiB)"`
	Time   time.Time `vfilter:"optional,field=time,doc=Format time (e.g. 2 hours ago)"`
	Comma  int64     `vfilter:"optional,field=comma,doc=Format integer with comma (e.g. 1,230)"`
}

type HumanizeFunction struct{}

func (self *HumanizeFunction) Call(ctx context.Context,
	scope vfilter.Scope,
	args *ordereddict.Dict) vfilter.Any {
	defer vql_subsystem.RegisterMonitor(ctx, "humanize", args)()

	arg := &HumanizeArgs{}
	err := arg_parser.ExtractArgsWithContext(ctx, scope, args, arg)
	if err != nil {
		scope.Log("Humanize: %s", err.Error())
		return false
	}

	if !arg.Time.IsZero() {
		return humanize.Time(arg.Time)
	}

	if arg.Comma != 0 {
		return humanize.Comma(arg.Comma)
	}

	if arg.Bytes != 0 {
		return humanize.Bytes(uint64(arg.Bytes))
	}

	return humanize.IBytes(uint64(arg.IBytes))
}

func (self HumanizeFunction) Info(scope vfilter.Scope, type_map *vfilter.TypeMap) *vfilter.FunctionInfo {
	return &vfilter.FunctionInfo{
		Name:    "humanize",
		Doc:     "Format items in human readable way.",
		ArgType: type_map.AddType(scope, &HumanizeArgs{}),
	}
}

func init() {
	vql_subsystem.RegisterFunction(&HumanizeFunction{})
}
