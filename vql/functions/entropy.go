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
	"math"

	"github.com/Velocidex/ordereddict"
	vql_subsystem "www.velocidex.com/golang/velociraptor/vql"
	"www.velocidex.com/golang/vfilter"
	"www.velocidex.com/golang/vfilter/arg_parser"
)

type entropy_args struct {
	String string `vfilter:"required,field=string"`
}

type Entropy struct{}

func (self *Entropy) Call(ctx context.Context,
	scope vfilter.Scope,
	args *ordereddict.Dict) vfilter.Any {

	defer vql_subsystem.RegisterMonitor(ctx, "entropy", args)()

	arg := &entropy_args{}
	err := arg_parser.ExtractArgsWithContext(ctx, scope, args, arg)
	if err != nil {
		scope.Log("entropy: %s", err.Error())
		return false
	}
	return float64(shannon(arg.String))
}

func shannon(value string) (bits float64) {
	frq := make(map[byte]float64)
	bv := []byte(value)
	//get frequency of characters
	for _, i := range bv {
		frq[i]++
	}

	var sum float64

	for _, v := range frq {
		f := float64(v) / float64(len(bv))
		sum += f * math.Log2(f)
	}
	bits = (math.Floor((sum*-1)*100) / 100)
	return
}

func (self Entropy) Info(scope vfilter.Scope, type_map *vfilter.TypeMap) *vfilter.FunctionInfo {
	return &vfilter.FunctionInfo{
		Name:    "entropy",
		Doc:     "Perform shannon entropy calculation on the input",
		ArgType: type_map.AddType(scope, &entropy_args{}),
	}
}

func init() {
	vql_subsystem.RegisterFunction(&Entropy{})
}
