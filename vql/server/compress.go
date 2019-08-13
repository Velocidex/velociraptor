// +build server_vql

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
package server

import (
	"compress/gzip"
	"context"
	"io"

	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	"www.velocidex.com/golang/velociraptor/file_store"
	vql_subsystem "www.velocidex.com/golang/velociraptor/vql"
	"www.velocidex.com/golang/vfilter"
)

type CompressArgs struct {
	VFSPath []string `vfilter:"required,field=path,doc=A VFS path to compress"`
}

type Compress struct{}

func (self *Compress) Call(ctx context.Context,
	scope *vfilter.Scope,
	args *vfilter.Dict) vfilter.Any {
	arg := &CompressArgs{}
	err := vfilter.ExtractArgs(scope, args, arg)
	if err != nil {
		scope.Log("compress: %s", err.Error())
		return vfilter.Null{}
	}

	any_config_obj, _ := scope.Resolve("server_config")
	config_obj, ok := any_config_obj.(*config_proto.Config)
	if !ok {
		scope.Log("Command can only run on the server")
		return vfilter.Null{}
	}

	result := []string{}
	file_store_factory := file_store.GetFileStore(config_obj)
	for _, path := range arg.VFSPath {
		func() {
			fd, err := file_store_factory.ReadFile(path)
			if err != nil {
				scope.Log("compress: %v", err)
				return
			}
			defer fd.Close()

			out_fd, err := file_store_factory.WriteFile(path + ".gz")
			if err != nil {
				scope.Log("compress: %v", err)
				return
			}
			defer out_fd.Close()

			zw := gzip.NewWriter(out_fd)
			defer zw.Close()

			zw.Name = path

			_, err = io.Copy(zw, fd)
			if err != nil {
				scope.Log("compress: %v", err)
				err2 := file_store_factory.Delete(path + ".gz")
				if err2 != nil {
					scope.Log("compress: cleaning up %v (%v)", err2, err)
				}
				return
			} else {
				err := file_store_factory.Delete(path)
				if err != nil {
					scope.Log("compress: %v", err)
				}
			}

			result = append(result, path)
		}()
	}

	return result
}

func (self Compress) Info(scope *vfilter.Scope, type_map *vfilter.TypeMap) *vfilter.FunctionInfo {
	return &vfilter.FunctionInfo{
		Name:    "compress",
		Doc:     "Compress a file in the server's FileStore. ",
		ArgType: type_map.AddType(scope, &CompressArgs{}),
	}
}

func init() {
	vql_subsystem.RegisterFunction(&Compress{})
}
