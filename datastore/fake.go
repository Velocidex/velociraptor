// A fake implementation of a datastore.
package datastore

import (
	"github.com/golang/protobuf/proto"
	"www.velocidex.com/golang/velociraptor/config"
	crypto_proto "www.velocidex.com/golang/velociraptor/crypto/proto"
)

type FakeDatastore struct {
	data map[string]map[string][]byte
}

func (self *FakeDatastore) Close() {
	self.data = make(map[string]map[string][]byte)
}

func (self *FakeDatastore) GetClientTasks(
	config_obj *config.Config,
	client_id string) ([]*crypto_proto.GrrMessage, error) {
	return []*crypto_proto.GrrMessage{}, nil
}

func (self *FakeDatastore) RemoveTasksFromClientQueue(
	config_obj *config.Config,
	client_id string,
	task_ids []uint64) error {
	return nil
}

func (self *FakeDatastore) QueueMessageForClient(
	config_obj *config.Config,
	client_id string,
	flow_id string,
	client_action string,
	message proto.Message,
	next_state uint64) error {
	return nil
}

func (self *FakeDatastore) SetSubjectData(
	config_obj *config.Config,
	urn string,
	data map[string][]byte) error {
	self.data[urn] = data
	return nil
}

func (self *FakeDatastore) GetSubjectData(
	config_obj *config.Config,
	urn string) (map[string][]byte, error) {
	data, pres := self.data[urn]
	if !pres {
		return make(map[string][]byte), nil
	}

	return data, nil
}

func init() {
	db := FakeDatastore{
		data: make(map[string]map[string][]byte),
	}

	RegisterImplementation("FakeDataStore", &db)
}
