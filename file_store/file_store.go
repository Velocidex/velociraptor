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

	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	"www.velocidex.com/golang/velociraptor/file_store/accessors"
	"www.velocidex.com/golang/velociraptor/file_store/api"
	"www.velocidex.com/golang/velociraptor/file_store/directory"
	"www.velocidex.com/golang/velociraptor/file_store/memory"
	"www.velocidex.com/golang/velociraptor/glob"
)

// GetFileStore selects an appropriate FileStore object based on
// config.
func GetFileStore(config_obj *config_proto.Config) api.FileStore {
	if config_obj.Datastore == nil {
		return nil
	}

	switch config_obj.Datastore.Implementation {
	case "Test":
		return memory.NewMemoryFileStore(config_obj)

	case "FileBaseDataStore", "MemcacheFileDataStore":
		return directory.NewDirectoryFileStore(config_obj)

	default:
		panic(fmt.Sprintf("Unsupported filestore %v",
			config_obj.Datastore.Implementation))
	}
}

// Gets an accessor that can access the file store.
func GetFileStoreFileSystemAccessor(
	config_obj *config_proto.Config) (glob.FileSystemAccessor, error) {
	if config_obj.Datastore == nil {
		return nil, errors.New("Datastore not configured")
	}

	switch config_obj.Datastore.Implementation {

	case "FileBaseDataStore", "MemcacheFileDataStore":
		return accessors.NewFileStoreFileSystemAccessor(
			config_obj, directory.NewDirectoryFileStore(config_obj)), nil

	case "Test":
		return accessors.NewFileStoreFileSystemAccessor(
			config_obj, memory.NewMemoryFileStore(config_obj)), nil

	}

	return nil, errors.New("Unknown file store implementation")
}
