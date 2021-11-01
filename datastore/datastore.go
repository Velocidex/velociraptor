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
// An interface into persistent data storage.
package datastore

import (
	"errors"
	"time"

	"google.golang.org/protobuf/proto"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	"www.velocidex.com/golang/velociraptor/file_store/api"
)

var (
	StopIteration = errors.New("StopIteration")
)

type SortingSense int

const (
	UNSORTED  = SortingSense(0)
	SORT_UP   = SortingSense(1)
	SORT_DOWN = SortingSense(2)
)

type DatastoreInfo struct {
	Name     string
	Modified time.Time
}

// When WalkFunc return StopIteration we exit the walk.
type WalkFunc func(urn api.DSPathSpec) error

type DataStore interface {
	// Reads a stored message from the datastore. If there is no
	// stored message at this URN, the function returns an
	// os.ErrNotExist error.
	GetSubject(
		config_obj *config_proto.Config,
		urn api.DSPathSpec,
		message proto.Message) error

	SetSubject(
		config_obj *config_proto.Config,
		urn api.DSPathSpec,
		message proto.Message) error

	DeleteSubject(
		config_obj *config_proto.Config,
		urn api.DSPathSpec) error

	// Lists all the children of a URN.
	ListChildren(
		config_obj *config_proto.Config,
		urn api.DSPathSpec) ([]api.DSPathSpec, error)

	Walk(config_obj *config_proto.Config,
		root api.DSPathSpec, walkFn WalkFunc) error

	Debug(config_obj *config_proto.Config)

	// Called to close all db handles etc. Not thread safe.
	Close()
}

func GetDB(config_obj *config_proto.Config) (DataStore, error) {
	if config_obj.Datastore == nil {
		return nil, errors.New("no datastore configured")
	}

	switch config_obj.Datastore.Implementation {
	case "FileBaseDataStore":
		if config_obj.Datastore.Location == "" {
			return nil, errors.New(
				"No Datastore_location is set in the config.")
		}

		return file_based_imp, nil

	case "Memcache":
		return memcache_imp, nil

	case "MemcacheFileDataStore":
		return memcache_file_imp, nil

	case "Test":
		return memcache_imp, nil

	default:
		return nil, errors.New("no datastore implementation " +
			config_obj.Datastore.Implementation)
	}
}
