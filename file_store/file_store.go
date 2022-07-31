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
	"fmt"
	"sync"

	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	"www.velocidex.com/golang/velociraptor/datastore"
	"www.velocidex.com/golang/velociraptor/file_store/api"
	"www.velocidex.com/golang/velociraptor/file_store/directory"
	"www.velocidex.com/golang/velociraptor/file_store/memcache"
	"www.velocidex.com/golang/velociraptor/file_store/memory"
)

var (
	fs_mu sync.Mutex

	// Key of org id to cache filestores
	g_impl = make(map[string]api.FileStore)
)

// GetFileStore selects an appropriate FileStore object based on
// config.
func GetFileStore(config_obj *config_proto.Config) api.FileStore {
	fs_mu.Lock()
	defer fs_mu.Unlock()

	// Maintain a different filestore for each org.
	impl, pres := g_impl[config_obj.OrgId]
	if pres {
		return impl
	}

	if config_obj.Datastore == nil {
		return nil
	}

	implementation, err := datastore.GetImplementationName(config_obj)
	if err != nil {
		panic(err)
	}

	res, _ := getImpl(implementation, config_obj)
	g_impl[config_obj.OrgId] = res
	return res
}

func getImpl(implementation string,
	config_obj *config_proto.Config) (api.FileStore, error) {
	switch implementation {
	case "Test":
		return memory.NewMemoryFileStore(config_obj), nil

	case "MemcacheFileDataStore", "RemoteFileDataStore":
		return memcache.NewMemcacheFileStore(config_obj), nil

	case "FileBaseDataStore", "ReadOnlyDataStore":
		return directory.NewDirectoryFileStore(config_obj), nil

	default:
		return nil, fmt.Errorf("Unsupported filestore %v", implementation)
	}
}

// Override the implementation
func OverrideFilestoreImplementation(
	config_obj *config_proto.Config, impl api.FileStore) {
	fs_mu.Lock()
	defer fs_mu.Unlock()

	g_impl[config_obj.OrgId] = impl
}

func SetGlobalFilestore(
	implementation string,
	config_obj *config_proto.Config) (err error) {
	fs_mu.Lock()
	defer fs_mu.Unlock()

	impl, err := getImpl(implementation, config_obj)
	if err != nil {
		return err
	}

	g_impl[config_obj.OrgId] = impl
	return nil
}

func Reset() {
	fs_mu.Lock()
	defer fs_mu.Unlock()

	g_impl = make(map[string]api.FileStore)
}
