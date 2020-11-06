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

	"github.com/golang/protobuf/proto"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	crypto_proto "www.velocidex.com/golang/velociraptor/crypto/proto"
)

type SortingSense int

const (
	UNSORTED  = SortingSense(0)
	SORT_UP   = SortingSense(1)
	SORT_DOWN = SortingSense(2)
)

type WalkFunc func(urn string) error

type DataStore interface {
	// Retrieve all the client's tasks.
	GetClientTasks(
		config_obj *config_proto.Config,
		client_id string,
		do_not_lease bool) ([]*crypto_proto.GrrMessage, error)

	UnQueueMessageForClient(
		config_obj *config_proto.Config,
		client_id string,
		message *crypto_proto.GrrMessage) error

	QueueMessageForClient(
		config_obj *config_proto.Config,
		client_id string,
		message *crypto_proto.GrrMessage) error

	GetSubject(
		config_obj *config_proto.Config,
		urn string,
		message proto.Message) error

	SetSubject(
		config_obj *config_proto.Config,
		urn string,
		message proto.Message) error

	DeleteSubject(
		config_obj *config_proto.Config,
		urn string) error

	// Lists all the children of a URN.
	ListChildren(
		config_obj *config_proto.Config,
		urn string,
		offset uint64, length uint64) ([]string, error)

	Walk(config_obj *config_proto.Config,
		root string, walkFn WalkFunc) error

	// Update the posting list index. Searching for any of the
	// keywords will return the entity urn.
	SetIndex(
		config_obj *config_proto.Config,
		index_urn string,
		entity string,
		keywords []string) error

	UnsetIndex(
		config_obj *config_proto.Config,
		index_urn string,
		entity string,
		keywords []string) error

	CheckIndex(
		config_obj *config_proto.Config,
		index_urn string,
		entity string,
		keywords []string) error

	SearchClients(
		config_obj *config_proto.Config,
		index_urn string,
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
		return file_based_imp, nil

	case "MySQL":
		return NewMySQLDataStore(config_obj)

	case "Test":
		mu.Lock()
		defer mu.Unlock()

		return gTestDatastore, nil

	default:
		return nil, errors.New("no datastore implementation " +
			config_obj.Datastore.Implementation)
	}
}
