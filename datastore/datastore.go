// An interface into persistent data storage.
package datastore

import (
	"errors"

	"github.com/golang/protobuf/proto"
	api_proto "www.velocidex.com/golang/velociraptor/api/proto"
	crypto_proto "www.velocidex.com/golang/velociraptor/crypto/proto"
)

var (
	implementations map[string]DataStore
)

type DataStore interface {
	// Retrieve all the client's tasks.
	GetClientTasks(
		config_obj *api_proto.Config,
		client_id string,
		do_not_lease bool) ([]*crypto_proto.GrrMessage, error)

	// Removes the task ids from the client queues.
	RemoveTasksFromClientQueue(
		config_obj *api_proto.Config,
		client_id string,
		task_ids []uint64) error

	QueueMessageForClient(
		config_obj *api_proto.Config,
		client_id string,
		flow_id string,
		client_action string,
		message proto.Message,
		next_state uint64) error

	GetSubject(
		config_obj *api_proto.Config,
		urn string,
		message proto.Message) error

	SetSubject(
		config_obj *api_proto.Config,
		urn string,
		message proto.Message) error

	DeleteSubject(
		config_obj *api_proto.Config,
		urn string) error

	// Lists all the children of a URN.
	ListChildren(
		config_obj *api_proto.Config,
		urn string,
		offset uint64, length uint64) ([]string, error)

	// Update the posting list index. Searching for any of the
	// keywords will return the entity urn.
	SetIndex(
		config_obj *api_proto.Config,
		index_urn string,
		entity string,
		keywords []string) error

	UnsetIndex(
		config_obj *api_proto.Config,
		index_urn string,
		entity string,
		keywords []string) error

	CheckIndex(
		config_obj *api_proto.Config,
		index_urn string,
		entity string,
		keywords []string) error

	SearchClients(
		config_obj *api_proto.Config,
		index_urn string,
		query string, query_type string,
		offset uint64, limit uint64) []string

	// Called to close all db handles etc. Not thread safe.
	Close()
}

func RegisterImplementation(name string, impl DataStore) {
	if implementations == nil {
		implementations = make(map[string]DataStore)
	}

	implementations[name] = impl
}

func GetImpl(name string) (DataStore, bool) {
	result, pres := implementations[name]
	return result, pres
}

func GetDB(config_obj *api_proto.Config) (DataStore, error) {
	db, pres := GetImpl(config_obj.Datastore.Implementation)
	if !pres {
		return nil, errors.New("no datastore implementation")
	}

	return db, nil
}
