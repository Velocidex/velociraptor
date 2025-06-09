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

	"github.com/Velocidex/ordereddict"
	vql_subsystem "www.velocidex.com/golang/velociraptor/vql"
	"www.velocidex.com/golang/vfilter"
	"www.velocidex.com/golang/vfilter/arg_parser"
)

type XorArgs struct {
	String string `vfilter:"required,field=string,doc=String to apply Xor"`
	Key    string `vfilter:"required,field=key,doc=Xor key."`
}

type Xor struct{}

func (self *Xor) Call(ctx context.Context,
	scope vfilter.Scope,
	args *ordereddict.Dict) vfilter.Any {

	defer vql_subsystem.RegisterMonitor(ctx, "xor", args)()

	arg := &XorArgs{}
	err := arg_parser.ExtractArgsWithContext(ctx, scope, args, arg)
	if err != nil {
		scope.Log("xor: %s", err.Error())
		return false
	}

	byte_output := xorbytes([]byte(arg.String), []byte(arg.Key))

	return string(byte_output)
}

func xorbytes(data, key []byte) (output []byte) {
	if len(key) == 0 {
		return data
	}

	for i := range data {
		output = append(output, data[i]^key[i%len(key)])
	}

	return output
}

func (self Xor) Info(scope vfilter.Scope, type_map *vfilter.TypeMap) *vfilter.FunctionInfo {
	return &vfilter.FunctionInfo{
		Name:    "xor",
		Doc:     "Apply xor to the string and key.",
		ArgType: type_map.AddType(scope, &XorArgs{}),
	}
}

func init() {
	vql_subsystem.RegisterFunction(&Xor{})
}
