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
	"strings"
	"sync"
	"time"

	"google.golang.org/protobuf/proto"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	crypto_proto "www.velocidex.com/golang/velociraptor/crypto/proto"
	"www.velocidex.com/golang/velociraptor/file_store/api"
)

var (
	mu sync.Mutex
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

type WalkFunc func(urn api.DSPathSpec) error
type ComponentWalkFunc func(components api.DSPathSpec) error

type DataStore interface {
	// Retrieve all the client's tasks.
	GetClientTasks(
		config_obj *config_proto.Config,
		client_id string,
		do_not_lease bool) ([]*crypto_proto.VeloMessage, error)

	UnQueueMessageForClient(
		config_obj *config_proto.Config,
		client_id string,
		message *crypto_proto.VeloMessage) error

	QueueMessageForClient(
		config_obj *config_proto.Config,
		client_id string,
		message *crypto_proto.VeloMessage) error

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
		urn api.DSPathSpec,
		offset uint64, length uint64) ([]api.DSPathSpec, error)

	Walk(config_obj *config_proto.Config,
		root api.DSPathSpec, walkFn WalkFunc) error

	// Update the posting list index. Searching for any of the
	// keywords will return the entity urn.
	SetIndex(
		config_obj *config_proto.Config,
		index_urn api.DSPathSpec,
		entity string,
		keywords []string) error

	UnsetIndex(
		config_obj *config_proto.Config,
		index_urn api.DSPathSpec,
		entity string,
		keywords []string) error

	CheckIndex(
		config_obj *config_proto.Config,
		index_urn api.DSPathSpec,
		entity string,
		keywords []string) error

	SearchClients(
		config_obj *config_proto.Config,
		index_urn api.DSPathSpec,
		query string, query_type string,
		offset uint64, limit uint64, sort SortingSense) []string

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

	case "Test":
		mu.Lock()
		defer mu.Unlock()

		// Sanitize the FilestoreDirectory parameter so we
		// have a consistent filename in the test datastore.
		config_obj.Datastore.Location = strings.TrimSuffix(
			config_obj.Datastore.Location, "/")

		return gTestDatastore, nil

	default:
		return nil, errors.New("no datastore implementation " +
			config_obj.Datastore.Implementation)
	}
}
