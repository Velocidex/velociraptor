/*
Velociraptor - Dig Deeper
Copyright (C) 2019-2024 Rapid7 Inc.

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

type similarityArgs struct {
	Set1 vfilter.Any `vfilter:"required,field=set1,doc=The first set to compare. *ordereddict.Dict vfilter.Any"`
	Set2 vfilter.Any `vfilter:"required,field=set2,doc=The second set to compare."`
}

type SimilarityFunction struct{}
		
func (self *SimilarityFunction) Call(
	ctx context.Context,
	scope vfilter.Scope,
	args *ordereddict.Dict) vfilter.Any {

	defer vql_subsystem.RegisterMonitor("similarity", args)()

	// Parse arguments using arg_parser
	arg := &similarityArgs{}
	err := arg_parser.ExtractArgsWithContext(ctx, scope, args, arg)
	if err != nil {
		scope.Log("similarity: %s", err.Error())
		return false
	}

	setA, okA := arg.Set1.(*ordereddict.Dict)
	setB, okB := arg.Set2.(*ordereddict.Dict)

	if !okA || !okB {
		if !okA { scope.Log("similarity: set1 parameter invalid") }
		if !okB { scope.Log("similarity: set2 parameter invalid") }
		return false
	}

	if scope.Eq(setA, setB){ return 1 }

	allKeys := ordereddict.NewDict()

	// Collect all unique keys from both sets
	for _, key := range setA.Keys() {
		allKeys.Set(key, nil)
	}
	for _, key := range setB.Keys() {
		allKeys.Set(key, nil)
	}

	// Calculate differences
	differences := 0
	for _, key := range allKeys.Keys() {
		valueA, okA := setA.Get(key)
		valueB, okB := setB.Get(key)
		//if !okA || !okB || valueA != valueB {
		if !okA || !okB || !scope.Eq(valueA, valueB) {
			differences++
		}
	}

	similarity := 1.0 - float64(differences)/float64(allKeys.Len())
	return similarity
}

func (self SimilarityFunction) Info(scope vfilter.Scope, type_map *vfilter.TypeMap) *vfilter.FunctionInfo {
	return &vfilter.FunctionInfo{
		Name:     "similarity",
		Doc:      "Compare two Dicts for similarity.",
		ArgType:  type_map.AddType(scope, &similarityArgs{}),
	}
}

func init() {
	vql_subsystem.RegisterFunction(&SimilarityFunction{})
}
