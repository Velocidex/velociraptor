/*
Velociraptor - Dig Deeper
Copyright (C) 2022 Velocidex Innovations.

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
	"bytes"
	"compress/gzip"
	"context"
	"io"

	"github.com/Velocidex/ordereddict"
	vql_subsystem "www.velocidex.com/golang/velociraptor/vql"
	"www.velocidex.com/golang/vfilter"
	"www.velocidex.com/golang/vfilter/arg_parser"
)

type GunzipArgs struct {
	String string `vfilter:"required,field=string,doc=Data to apply Gunzip"`
}

type Gunzip struct{}

func (self *Gunzip) Call(ctx context.Context,
	scope vfilter.Scope,
	args *ordereddict.Dict) vfilter.Any {

	defer vql_subsystem.RegisterMonitor(ctx, "gunzip", args)()

	arg := &GunzipArgs{}
	err := arg_parser.ExtractArgsWithContext(ctx, scope, args, arg)
	if err != nil {
		scope.Log("Gunzip: %s", err.Error())
		return false
	}

	b := bytes.NewBuffer([]byte(arg.String))
	var r io.Reader
	r, err = gzip.NewReader(b)

	if err != nil {
		scope.Log("Gunzip: %s", err.Error())
		return false
	}

	var resB bytes.Buffer
	_, err = resB.ReadFrom(r)

	if err != nil {
		scope.Log("Gunzip: %s", err.Error())
		return false
	}

	return string(resB.Bytes())
}

func (self Gunzip) Info(scope vfilter.Scope, type_map *vfilter.TypeMap) *vfilter.FunctionInfo {
	return &vfilter.FunctionInfo{
		Name:    "gunzip",
		Doc:     "Uncompress a gzip-compressed block of data.",
		ArgType: type_map.AddType(scope, &GunzipArgs{}),
	}
}

func init() {
	vql_subsystem.RegisterFunction(&Gunzip{})
}
