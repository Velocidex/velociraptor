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
	urn string, timestamp int64,
	data map[string][]byte) error {
	self.data[urn] = data
	return nil
}
func (self *FakeDatastore) GetSubjectAttribute(
	config_obj *config.Config,
	urn string, attr string) ([]byte, bool) {
	data, pres := self.data[urn]
	if pres {
		value, pres := data[attr]
		if pres {
			return value, true
		}
	}

	return nil, false
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

func (self *FakeDatastore) SetIndex(
	config_obj *config.Config,
	index_urn string,
	entity string,
	keywords []string) error {

	data := make(map[string][]byte)
	data[entity] = []byte("X")

	for _, keyword := range keywords {
		self.data[index_urn+keyword] = data
	}

	return nil
}

func (self *FakeDatastore) SearchClients(
	config_obj *config.Config,
	index_urn string,
	query string,
	start uint64, end uint64) []string {
	return []string{}
}

func init() {
	db := FakeDatastore{
		data: make(map[string]map[string][]byte),
	}

	RegisterImplementation("FakeDataStore", &db)
}
