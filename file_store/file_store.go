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

	"www.velocidex.com/golang/velociraptor/accessors"
	file_store_accessor "www.velocidex.com/golang/velociraptor/accessors/file_store"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	"www.velocidex.com/golang/velociraptor/datastore"
	"www.velocidex.com/golang/velociraptor/file_store/api"
	"www.velocidex.com/golang/velociraptor/file_store/directory"
	"www.velocidex.com/golang/velociraptor/file_store/memcache"
	"www.velocidex.com/golang/velociraptor/file_store/memory"
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

	implementation, err := datastore.GetImplementationName(config_obj)
	if err != nil {
		panic(err)
	}

	res, _ := getImpl(implementation, config_obj)
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

	case "FileBaseDataStore", "ReadOnlyDataStore":
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
	config_obj *config_proto.Config) (accessors.FileSystemAccessor, error) {

	fs_mu.Lock()
	defer fs_mu.Unlock()

	if g_impl != nil {
		return file_store_accessor.NewFileStoreFileSystemAccessor(
			config_obj, g_impl), nil
	}

	if config_obj.Datastore == nil {
		return nil, errors.New("Datastore not configured")
	}

	implementation, err := datastore.GetImplementationName(config_obj)
	if err != nil {
		return nil, err
	}

	switch implementation {

	case "MemcacheFileDataStore":
		return file_store_accessor.NewFileStoreFileSystemAccessor(
			config_obj, memcache_imp), nil

	case "FileBaseDataStore", "RemoteFileDataStore", "ReadOnlyDataStore":
		return file_store_accessor.NewFileStoreFileSystemAccessor(
			config_obj, directory_imp), nil

	case "Test":
		return file_store_accessor.NewFileStoreFileSystemAccessor(
			config_obj, memory_imp), nil

	}

	return nil, errors.New("Unknown file store implementation")
}

// Override the implementation
func OverrideFilestoreImplementation(impl api.FileStore) {
	fs_mu.Lock()
	defer fs_mu.Unlock()

	g_impl = impl
}

func SetGlobalFilestore(
	implementation string,
	config_obj *config_proto.Config) (err error) {
	fs_mu.Lock()
	defer fs_mu.Unlock()

	g_impl, err = getImpl(implementation, config_obj)
	return err
}

func Reset() {
	fs_mu.Lock()
	defer fs_mu.Unlock()

	memory_imp = nil
	memcache_imp = nil
	directory_imp = nil
	g_impl = nil
}
