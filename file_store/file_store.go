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
package file_store

import (
	"context"
	"fmt"
	"sync"

	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	"www.velocidex.com/golang/velociraptor/datastore"
	"www.velocidex.com/golang/velociraptor/file_store/api"
	"www.velocidex.com/golang/velociraptor/file_store/directory"
	"www.velocidex.com/golang/velociraptor/file_store/memcache"
	"www.velocidex.com/golang/velociraptor/file_store/memory"
	"www.velocidex.com/golang/velociraptor/utils"
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
	org_id := utils.NormalizedOrgId(config_obj.OrgId)

	impl, pres := g_impl[org_id]
	if pres && impl != nil {
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
	g_impl[org_id] = res
	return res
}

func getImpl(implementation string,
	config_obj *config_proto.Config) (api.FileStore, error) {
	switch implementation {
	case "Test":
		return memory.NewMemoryFileStore(config_obj), nil

	case "MemcacheFileDataStore", "RemoteFileDataStore":
		return memcache.NewMemcacheFileStore(
			context.Background(), config_obj), nil

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

	org_id := utils.NormalizedOrgId(config_obj.OrgId)
	g_impl[org_id] = impl
}

// Used by tests to reset global state.
func ClearGlobalFilestore() {
	fs_mu.Lock()
	defer fs_mu.Unlock()

	g_impl = make(map[string]api.FileStore)
}

func SetGlobalFilestore(
	implementation string,
	config_obj *config_proto.Config) (err error) {
	fs_mu.Lock()
	defer fs_mu.Unlock()

	org_id := utils.NormalizedOrgId(config_obj.OrgId)

	if implementation == "clear" {
		delete(g_impl, org_id)
		return nil
	}

	// Nothing to update, the filestore is already set correctly.
	current_impl, pres := g_impl[org_id]
	if pres {
		_, ok := current_impl.(*memcache.MemcacheFileStore)
		if ok && implementation == "MemcacheFileDataStore" {
			return nil
		}
	}

	impl, err := getImpl(implementation, config_obj)
	if err != nil {
		return err
	}

	g_impl[org_id] = impl
	return nil
}
