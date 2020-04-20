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
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	"www.velocidex.com/golang/velociraptor/constants"
	"www.velocidex.com/golang/velociraptor/datastore"
	"www.velocidex.com/golang/velociraptor/file_store"
	vql_subsystem "www.velocidex.com/golang/velociraptor/vql"
	"www.velocidex.com/golang/vfilter"
)

type DeleteFileStoreArgs struct {
	VFSPath string `vfilter:"required,field=path,doc=A VFS path to remove"`
}

type DeleteFileStore struct{}

func (self *DeleteFileStore) Call(ctx context.Context,
	scope *vfilter.Scope,
	args *ordereddict.Dict) vfilter.Any {
	arg := &DeleteFileStoreArgs{}

	err := vql_subsystem.CheckAccess(scope, acls.SERVER_ADMIN)
	if err != nil {
		scope.Log("flows: %s", err)
		return vfilter.Null{}
	}

	err = vfilter.ExtractArgs(scope, args, arg)
	if err != nil {
		scope.Log("file_store_delete: %s", err.Error())
		return vfilter.Null{}
	}

	any_config_obj, _ := scope.Resolve(constants.SCOPE_SERVER_CONFIG)
	config_obj, ok := any_config_obj.(*config_proto.Config)
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
		err = db.DeleteSubject(config_obj, strings.TrimSuffix(arg.VFSPath, ".db"))
	} else {
		err = file_store_factory.Delete(arg.VFSPath)
	}

	if err != nil {
		scope.Log("file_store_delete: %s", err.Error())
		return vfilter.Null{}
	}
	return arg.VFSPath
}

func (self DeleteFileStore) Info(scope *vfilter.Scope, type_map *vfilter.TypeMap) *vfilter.FunctionInfo {
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
	scope *vfilter.Scope,
	args *ordereddict.Dict) vfilter.Any {
	arg := &FileStoreArgs{}
	err := vfilter.ExtractArgs(scope, args, arg)
	if err != nil {
		scope.Log("file_store: %s", err.Error())
		return vfilter.Null{}
	}

	any_config_obj, _ := scope.Resolve(constants.SCOPE_SERVER_CONFIG)
	config_obj, ok := any_config_obj.(*config_proto.Config)
	if !ok {
		scope.Log("Command can only run on the server")
		return vfilter.Null{}
	}

	result := []string{}
	file_store_factory := file_store.GetFileStore(config_obj)
	for _, path := range arg.VFSPath {
		file_path := file_store_factory.(*file_store.DirectoryFileStore).
			FilenameToFileStorePath(path)
		result = append(result, file_path)
	}

	return result
}

func (self FileStore) Info(scope *vfilter.Scope, type_map *vfilter.TypeMap) *vfilter.FunctionInfo {
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
