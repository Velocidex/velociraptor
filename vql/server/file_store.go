//go:build server_vql
// +build server_vql

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
	"context"
	"errors"
	"os"
	"strings"

	"github.com/Velocidex/ordereddict"
	"www.velocidex.com/golang/velociraptor/accessors"
	"www.velocidex.com/golang/velociraptor/acls"
	"www.velocidex.com/golang/velociraptor/datastore"
	"www.velocidex.com/golang/velociraptor/file_store"
	"www.velocidex.com/golang/velociraptor/file_store/api"
	"www.velocidex.com/golang/velociraptor/file_store/path_specs"
	"www.velocidex.com/golang/velociraptor/logging"
	"www.velocidex.com/golang/velociraptor/paths"
	"www.velocidex.com/golang/velociraptor/services"
	"www.velocidex.com/golang/velociraptor/vql"
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

	err = services.RequireFrontend()
	if err != nil {
		scope.Log("file_store_delete: %v", err)
		return vfilter.Null{}
	}

	config_obj, ok := vql_subsystem.GetServerConfig(scope)
	if !ok {
		scope.Log("file_store_delete: Command can only run on the server")
		return vfilter.Null{}
	}

	db, err := datastore.GetDB(config_obj)
	if err != nil {
		return vfilter.Null{}
	}

	file_store_factory := file_store.GetFileStore(config_obj)

	vfs_path := arg.VFSPath.Reduce(ctx)
	principal := vql_subsystem.GetPrincipal(scope)
	err = services.LogAudit(ctx,
		config_obj, principal, "file_store_delete",
		ordereddict.NewDict().Set("vfs", vfs_path))
	if err != nil {
		logger := logging.GetLogger(config_obj, &logging.FrontendComponent)
		logger.Error("<red>DeleteFileStoreArgs</> %v %v", principal, vfs_path)
	}

	switch t := vfs_path.(type) {
	case *path_specs.DSPathSpec:
		err = db.DeleteSubject(config_obj, t)

	case path_specs.DSPathSpec:
		err = db.DeleteSubject(config_obj, t)

	case *path_specs.FSPathSpec:
		err = file_store_factory.Delete(t)

	case path_specs.FSPathSpec:
		err = file_store_factory.Delete(t)

	case *accessors.OSPath:
		path_spec := path_specs.NewSafeFilestorePath(t.Components...).
			SetType(api.PATH_TYPE_FILESTORE_ANY)
		err = file_store_factory.Delete(path_spec)

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
		Name:     "file_store_delete",
		Doc:      "Delete file store paths into full filesystem paths. ",
		ArgType:  type_map.AddType(scope, &DeleteFileStoreArgs{}),
		Metadata: vql.VQLMetadata().Permissions(acls.SERVER_ADMIN).Build(),
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

	err = services.RequireFrontend()
	if err != nil {
		scope.Log("file_store: %v", err)
		return vfilter.Null{}
	}

	config_obj, ok := vql_subsystem.GetServerConfig(scope)
	if !ok {
		scope.Log("file_store: Command can only run on the server")
		return vfilter.Null{}
	}

	db, err := datastore.GetDB(config_obj)
	if err != nil {
		scope.Log("file_store: %v", err)
		return vfilter.Null{}
	}

	vfs_path := arg.VFSPath.Reduce(ctx)
	switch t := vfs_path.(type) {
	case *path_specs.FSPathSpec:
		return datastore.AsFilestoreFilename(db, config_obj, t)

	case path_specs.FSPathSpec:
		return datastore.AsFilestoreFilename(db, config_obj, t)

	case *path_specs.DSPathSpec:
		return datastore.AsDatastoreFilename(db, config_obj, t)

	case path_specs.DSPathSpec:
		return datastore.AsDatastoreFilename(db, config_obj, t)

	case *accessors.OSPath:
		return datastore.AsFilestoreFilename(db, config_obj,
			path_specs.NewUnsafeFilestorePath(t.Components...))

	case string:
		// Things that produce strings normally encode the path spec
		// with a prefix to let us know if this is a data store path
		// or a filestore path..
		if strings.HasPrefix(t, "ds:") {
			return datastore.AsDatastoreFilename(db, config_obj,
				paths.DSPathSpecFromClientPath(
					strings.TrimPrefix(t, "ds:")))
		}

		return datastore.AsFilestoreFilename(db, config_obj,
			paths.FSPathSpecFromClientPath(
				strings.TrimPrefix(t, "fs:")))
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
