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
package server

import (
	"compress/gzip"
	"context"
	"os"
	"strings"

	"github.com/Velocidex/ordereddict"
	"www.velocidex.com/golang/velociraptor/accessors/file"
	"www.velocidex.com/golang/velociraptor/acls"
	"www.velocidex.com/golang/velociraptor/utils"
	"www.velocidex.com/golang/velociraptor/vql"
	vql_subsystem "www.velocidex.com/golang/velociraptor/vql"
	"www.velocidex.com/golang/vfilter"
	"www.velocidex.com/golang/vfilter/arg_parser"
)

type CompressArgs struct {
	Path   string `vfilter:"required,field=path,doc=A path to compress"`
	Output string `vfilter:"required,field=output,doc=A path to write the output - default is the path with a .gz extension"`
}

type Compress struct{}

func (self *Compress) Call(ctx context.Context,
	scope vfilter.Scope,
	args *ordereddict.Dict) vfilter.Any {

	err := vql_subsystem.CheckAccess(scope, acls.FILESYSTEM_WRITE, acls.FILESYSTEM_READ)
	if err != nil {
		scope.Log("compress: %v", err)
		return vfilter.Null{}
	}

	arg := &CompressArgs{}
	err = arg_parser.ExtractArgsWithContext(ctx, scope, args, arg)
	if err != nil {
		scope.Log("compress: %s", err.Error())
		return vfilter.Null{}
	}

	// Are we allowed to write there?
	err = file.CheckPath(arg.Path)
	if err != nil {
		scope.Log("compress: %s", err.Error())
		return vfilter.Null{}
	}

	fd, err := os.Open(arg.Path)
	if err != nil {
		scope.Log("compress: %v", err)
		return vfilter.Null{}
	}
	defer fd.Close()

	out_fd, err := os.OpenFile(arg.Output,
		os.O_RDWR|os.O_CREATE|os.O_TRUNC, 0660)
	if err != nil {
		scope.Log("compress: %v", err)
		return vfilter.Null{}
	}
	defer out_fd.Close()

	zw := gzip.NewWriter(out_fd)
	defer zw.Close()

	zw.Name = strings.TrimPrefix(arg.Path, "/")

	_, err = utils.Copy(ctx, zw, fd)
	if err != nil {
		scope.Log("compress: %v", err)
		err2 := os.Remove(arg.Output)
		if err2 != nil {
			scope.Log("compress: cleaning up %v (%v)", err2, err)
		}
		return vfilter.Null{}
	}

	return arg.Output
}

func (self Compress) Info(scope vfilter.Scope, type_map *vfilter.TypeMap) *vfilter.FunctionInfo {
	return &vfilter.FunctionInfo{
		Name:     "compress",
		Doc:      "Compress a file in the server's FileStore. ",
		ArgType:  type_map.AddType(scope, &CompressArgs{}),
		Metadata: vql.VQLMetadata().Permissions(acls.FILESYSTEM_WRITE, acls.FILESYSTEM_READ).Build(),
	}
}

func init() {
	vql_subsystem.RegisterFunction(&Compress{})
}
