package server

import (
	"context"

	api_proto "www.velocidex.com/golang/velociraptor/api/proto"
	"www.velocidex.com/golang/velociraptor/file_store"
	vql_subsystem "www.velocidex.com/golang/velociraptor/vql"
	"www.velocidex.com/golang/vfilter"
)

type FileStoreArgs struct {
	VFSPath []string `vfilter:"required,field=path"`
}

type FileStore struct{}

func (self *FileStore) Call(ctx context.Context,
	scope *vfilter.Scope,
	args *vfilter.Dict) vfilter.Any {
	arg := &FileStoreArgs{}
	err := vfilter.ExtractArgs(scope, args, arg)
	if err != nil {
		scope.Log("file_store: %s", err.Error())
		return vfilter.Null{}
	}

	any_config_obj, _ := scope.Resolve("server_config")
	config_obj, ok := any_config_obj.(*api_proto.Config)
	if !ok {
		scope.Log("Command can only run on the server")
		return vfilter.Null{}
	}

	result := []string{}
	file_store_factory := file_store.GetFileStore(config_obj)
	for _, path := range arg.VFSPath {
		file_path, err := file_store_factory.(*file_store.DirectoryFileStore).
			FilenameToFileStorePath(path)
		if err == nil {
			result = append(result, file_path)
		}
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
}
