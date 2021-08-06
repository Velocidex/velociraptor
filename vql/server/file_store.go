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
	"context"
	"strings"

	"github.com/Velocidex/ordereddict"
	"www.velocidex.com/golang/velociraptor/acls"
	"www.velocidex.com/golang/velociraptor/datastore"
	"www.velocidex.com/golang/velociraptor/file_store"
	"www.velocidex.com/golang/velociraptor/file_store/path_specs"
	"www.velocidex.com/golang/velociraptor/utils"
	vql_subsystem "www.velocidex.com/golang/velociraptor/vql"
	"www.velocidex.com/golang/vfilter"
	"www.velocidex.com/golang/vfilter/arg_parser"
)

type DeleteFileStoreArgs struct {
	VFSPath string `vfilter:"required,field=path,doc=A VFS path to remove"`
}

type DeleteFileStore struct{}

func (self *DeleteFileStore) Call(ctx context.Context,
	scope vfilter.Scope,
	args *ordereddict.Dict) vfilter.Any {
	arg := &DeleteFileStoreArgs{}

	err := vql_subsystem.CheckAccess(scope, acls.SERVER_ADMIN)
	if err != nil {
		scope.Log("flows: %s", err)
		return vfilter.Null{}
	}

	err = arg_parser.ExtractArgsWithContext(ctx, scope, args, arg)
	if err != nil {
		scope.Log("file_store_delete: %s", err.Error())
		return vfilter.Null{}
	}

	config_obj, ok := vql_subsystem.GetServerConfig(scope)
	if !ok {
		scope.Log("Command can only run on the server")
		return vfilter.Null{}
	}

	db, err := datastore.GetDB(config_obj)
	if err != nil {
		return vfilter.Null{}
	}

	file_store_factory := file_store.GetFileStore(config_obj)
	if strings.HasSuffix(arg.VFSPath, "db") {
		// This is a data store path
		pathspec := path_specs.NewUnsafeDatastorePath(
			utils.SplitComponents(
				strings.TrimSuffix(arg.VFSPath, ".db"))...)
		err = db.DeleteSubject(config_obj, pathspec)
	} else {

		// This is a file store path.
		pathspec := path_specs.NewUnsafeFilestorePath(
			utils.SplitComponents(arg.VFSPath)...)
		err = file_store_factory.Delete(pathspec)
	}

	if err != nil {
		scope.Log("file_store_delete: %s", err.Error())
		return vfilter.Null{}
	}
	return arg.VFSPath
}

func (self DeleteFileStore) Info(scope vfilter.Scope, type_map *vfilter.TypeMap) *vfilter.FunctionInfo {
	return &vfilter.FunctionInfo{
		Name:    "file_store_delete",
		Doc:     "Delete file store paths into full filesystem paths. ",
		ArgType: type_map.AddType(scope, &DeleteFileStoreArgs{}),
	}
}

type FileStoreArgs struct {
	VFSPath []string `vfilter:"required,field=path,doc=A VFS path to convert"`
}

type FileStore struct{}

func (self *FileStore) Call(ctx context.Context,
	scope vfilter.Scope,
	args *ordereddict.Dict) vfilter.Any {
	arg := &FileStoreArgs{}
	err := arg_parser.ExtractArgsWithContext(ctx, scope, args, arg)
	if err != nil {
		scope.Log("file_store: %s", err.Error())
		return vfilter.Null{}
	}

	config_obj, ok := vql_subsystem.GetServerConfig(scope)
	if !ok {
		scope.Log("Command can only run on the server")
		return vfilter.Null{}
	}

	result := []string{}
	for _, path := range arg.VFSPath {
		pathspec := path_specs.NewUnsafeFilestorePath(
			utils.SplitComponents(path)...)
		result = append(result, pathspec.AsFilestoreFilename(config_obj))
	}

	return result
}

func (self FileStore) Info(scope vfilter.Scope, type_map *vfilter.TypeMap) *vfilter.FunctionInfo {
	return &vfilter.FunctionInfo{
		Name:    "file_store",
		Doc:     "Resolves file store paths into full filesystem paths. ",
		ArgType: type_map.AddType(scope, &FileStoreArgs{}),
	}
}

func init() {
	vql_subsystem.RegisterFunction(&FileStore{})
	vql_subsystem.RegisterFunction(&DeleteFileStore{})
}
