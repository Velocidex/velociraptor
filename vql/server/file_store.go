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
	"errors"
	"os"
	"strings"

	"github.com/Velocidex/ordereddict"
	"www.velocidex.com/golang/velociraptor/accessors"
	"www.velocidex.com/golang/velociraptor/acls"
	"www.velocidex.com/golang/velociraptor/datastore"
	"www.velocidex.com/golang/velociraptor/file_store"
	"www.velocidex.com/golang/velociraptor/file_store/path_specs"
	"www.velocidex.com/golang/velociraptor/paths"
	vql_subsystem "www.velocidex.com/golang/velociraptor/vql"
	"www.velocidex.com/golang/vfilter"
	"www.velocidex.com/golang/vfilter/arg_parser"
	"www.velocidex.com/golang/vfilter/types"
)

type DeleteFileStoreArgs struct {
	VFSPath types.LazyExpr `vfilter:"required,field=path,doc=A VFS path to remove"`
}

type DeleteFileStore struct{}

func (self *DeleteFileStore) Call(ctx context.Context,
	scope vfilter.Scope,
	args *ordereddict.Dict) vfilter.Any {
	arg := &DeleteFileStoreArgs{}

	err := vql_subsystem.CheckAccess(scope, acls.SERVER_ADMIN)
	if err != nil {
		scope.Log("file_store_delete: %v", err)
		return vfilter.Null{}
	}

	err = arg_parser.ExtractArgsWithContext(ctx, scope, args, arg)
	if err != nil {
		scope.Log("file_store_delete: %v", err)
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

	vfs_path := arg.VFSPath.Reduce(ctx)
	switch t := vfs_path.(type) {
	case *path_specs.DSPathSpec:
		err = db.DeleteSubject(config_obj, t)

	case path_specs.DSPathSpec:
		err = db.DeleteSubject(config_obj, t)

	case *path_specs.FSPathSpec:
		err = file_store_factory.Delete(t)

	case path_specs.FSPathSpec:
		err = file_store_factory.Delete(t)

	case string:
		// Things that produce strings normally encode the path spec
		// with a prefix to let us know if this is a data store path
		// or a filestore path..
		if strings.HasPrefix(t, "ds:") {
			path_spec := paths.DSPathSpecFromClientPath(
				strings.TrimPrefix(t, "ds:"))
			err = db.DeleteSubject(config_obj, path_spec)
		} else {
			path_spec := paths.FSPathSpecFromClientPath(
				strings.TrimPrefix(t, "fs:"))
			err = file_store_factory.Delete(path_spec)
		}

	default:
		scope.Log("file_store_delete: Unsupported VFS path type %T", vfs_path)
		return vfilter.Null{}
	}

	if err != nil && !errors.Is(err, os.ErrNotExist) {
		scope.Log("file_store_delete: %v", err)
		return vfilter.Null{}
	}

	return vfs_path
}

func (self DeleteFileStore) Info(scope vfilter.Scope, type_map *vfilter.TypeMap) *vfilter.FunctionInfo {
	return &vfilter.FunctionInfo{
		Name:    "file_store_delete",
		Doc:     "Delete file store paths into full filesystem paths. ",
		ArgType: type_map.AddType(scope, &DeleteFileStoreArgs{}),
	}
}

type FileStoreArgs struct {
	VFSPath types.LazyExpr `vfilter:"required,field=path,doc=A VFS path to convert"`
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

	vfs_path := arg.VFSPath.Reduce(ctx)
	switch t := vfs_path.(type) {
	case *path_specs.FSPathSpec:
		return t.AsFilestoreFilename(config_obj)

	case path_specs.FSPathSpec:
		return t.AsFilestoreFilename(config_obj)

	case *path_specs.DSPathSpec:
		return t.AsDatastoreFilename(config_obj)

	case path_specs.DSPathSpec:
		return t.AsDatastoreFilename(config_obj)

	case *accessors.OSPath:
		return path_specs.NewUnsafeFilestorePath(t.Components...).AsFilestoreFilename(config_obj)

	case string:
		// Things that produce strings normally encode the path spec
		// with a prefix to let us know if this is a data store path
		// or a filestore path..
		if strings.HasPrefix(t, "ds:") {
			return paths.DSPathSpecFromClientPath(
				strings.TrimPrefix(t, "ds:")).
				AsDatastoreFilename(config_obj)
		}

		return paths.FSPathSpecFromClientPath(
			strings.TrimPrefix(t, "fs:")).
			AsFilestoreFilename(config_obj)
	}

	return vfilter.Null{}
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
