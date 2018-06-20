// An interface into persistent data storage.
package datastore

import (
	"errors"
	"github.com/golang/protobuf/proto"
	"www.velocidex.com/golang/velociraptor/config"
	crypto_proto "www.velocidex.com/golang/velociraptor/crypto/proto"
)

var (
	implementations map[string]DataStore
)

type DataStore interface {
	// Retrieve all the client's tasks.
	GetClientTasks(
		config_obj *config.Config,
		client_id string) ([]*crypto_proto.GrrMessage, error)

	// Removes the task ids from the client queues.
	RemoveTasksFromClientQueue(
		config_obj *config.Config,
		client_id string,
		task_ids []uint64) error

	QueueMessageForClient(
		config_obj *config.Config,
		client_id string,
		flow_id string,
		client_action string,
		message proto.Message,
		next_state uint64) error

	// Just grab the whole data of the AFF4 object.
	GetSubjectData(
		config_obj *config.Config,
		urn string) (map[string][]byte, error)

	// Just grab the whole data of the AFF4 object.
	SetSubjectData(
		config_obj *config.Config,
		urn string,
		data map[string][]byte) error

	// Update the posting list index. Searching for any of the
	// keywords will return the entity urn.
	SetIndex(
		config_obj *config.Config,
		index_urn string,
		entity string,
		keywords []string) error

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

func GetDB(config_obj *config.Config) (DataStore, error) {
	db, pres := GetImpl(*config_obj.Datastore_implementation)
	if !pres {
		return nil, errors.New("No datastore implementation")
	}

	return db, nil
}
