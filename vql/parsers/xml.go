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
package parsers

import (
	"context"

	"github.com/Velocidex/ordereddict"
	"github.com/clbanning/mxj"
	"www.velocidex.com/golang/velociraptor/accessors"
	vql_subsystem "www.velocidex.com/golang/velociraptor/vql"
	vfilter "www.velocidex.com/golang/vfilter"
	"www.velocidex.com/golang/vfilter/arg_parser"
)

type _ParseXMLFunctionArgs struct {
	File     string `vfilter:"required,field=file,doc=XML file to open."`
	Accessor string `vfilter:"optional,field=accessor,doc=The accessor to use"`
}
type _ParseXMLFunction struct{}

func (self _ParseXMLFunction) Call(
	ctx context.Context,
	scope vfilter.Scope,
	args *ordereddict.Dict) vfilter.Any {
	arg := &_ParseXMLFunctionArgs{}
	err := arg_parser.ExtractArgsWithContext(ctx, scope, args, arg)
	if err != nil {
		scope.Log("parse_xml: %s", err.Error())
		return vfilter.Null{}
	}

	err = vql_subsystem.CheckFilesystemAccess(scope, arg.Accessor)
	if err != nil {
		scope.Log("parse_xml: %s", err)
		return vfilter.Null{}
	}

	accessor, err := accessors.GetAccessor(arg.Accessor, scope)
	if err != nil {
		scope.Log("parse_xml: %v", err)
		return vfilter.Null{}
	}
	file, err := accessor.Open(arg.File)
	if err != nil {
		scope.Log("Unable to open file %s", arg.File)
		return vfilter.Null{}
	}
	defer file.Close()

	mxj.SetAttrPrefix("Attr")
	result, err := mxj.NewMapXmlReader(file)
	if err != nil {
		scope.Log("NewMapXmlReader: %v", err)
		return vfilter.Null{}
	}

	return result.Old()
}

func (self _ParseXMLFunction) Info(scope vfilter.Scope, type_map *vfilter.TypeMap) *vfilter.FunctionInfo {
	return &vfilter.FunctionInfo{
		Name:    "parse_xml",
		Doc:     "Parse an XML document into a map.",
		ArgType: type_map.AddType(scope, &_ParseXMLFunctionArgs{}),
	}
}

func init() {
	vql_subsystem.RegisterFunction(&_ParseXMLFunction{})
}
