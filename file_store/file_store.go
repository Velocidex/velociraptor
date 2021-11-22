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
package file_store

import (
	"errors"
	"fmt"
	"sync"

	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	"www.velocidex.com/golang/velociraptor/file_store/accessors"
	"www.velocidex.com/golang/velociraptor/file_store/api"
	"www.velocidex.com/golang/velociraptor/file_store/directory"
	"www.velocidex.com/golang/velociraptor/file_store/memcache"
	"www.velocidex.com/golang/velociraptor/file_store/memory"
	"www.velocidex.com/golang/velociraptor/glob"
)

var (
	fs_mu         sync.Mutex
	memory_imp    *memory.MemoryFileStore
	memcache_imp  *memcache.MemcacheFileStore
	directory_imp *directory.DirectoryFileStore

	g_impl api.FileStore
)

// GetFileStore selects an appropriate FileStore object based on
// config.
func GetFileStore(config_obj *config_proto.Config) api.FileStore {
	fs_mu.Lock()
	defer fs_mu.Unlock()

	if g_impl != nil {
		return g_impl
	}

	if config_obj.Datastore == nil {
		return nil
	}

	res, _ := getImpl(config_obj.Datastore.Implementation, config_obj)
	return res
}

func getImpl(implementation string,
	config_obj *config_proto.Config) (api.FileStore, error) {
	switch implementation {
	case "Test":
		if memory_imp == nil {
			memory_imp = memory.NewMemoryFileStore(config_obj)
		}
		return memory_imp, nil

	case "MemcacheFileDataStore", "RemoteFileDataStore":
		if memcache_imp == nil {
			memcache_imp = memcache.NewMemcacheFileStore(config_obj)
		}
		return memcache_imp, nil

	case "FileBaseDataStore":
		if directory_imp == nil {
			directory_imp = directory.NewDirectoryFileStore(config_obj)
		}
		return directory_imp, nil

	default:
		return nil, fmt.Errorf("Unsupported filestore %v", implementation)
	}
}

// Gets an accessor that can access the file store.
func GetFileStoreFileSystemAccessor(
	config_obj *config_proto.Config) (glob.FileSystemAccessor, error) {

	fs_mu.Lock()
	defer fs_mu.Unlock()

	if g_impl != nil {
		return accessors.NewFileStoreFileSystemAccessor(
			config_obj, g_impl), nil
	}

	if config_obj.Datastore == nil {
		return nil, errors.New("Datastore not configured")
	}

	switch config_obj.Datastore.Implementation {

	case "MemcacheFileDataStore":
		return accessors.NewFileStoreFileSystemAccessor(
			config_obj, memcache_imp), nil

	case "FileBaseDataStore", "RemoteFileDataStore":
		return accessors.NewFileStoreFileSystemAccessor(
			config_obj, directory_imp), nil

	case "Test":
		return accessors.NewFileStoreFileSystemAccessor(
			config_obj, memory_imp), nil

	}

	return nil, errors.New("Unknown file store implementation")
}

func SetGlobalFilestore(
	implementation string,
	config_obj *config_proto.Config) (err error) {
	fs_mu.Lock()
	defer fs_mu.Unlock()

	g_impl, err = getImpl(implementation, config_obj)
	return err
}
