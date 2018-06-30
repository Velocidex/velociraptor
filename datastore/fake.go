// A fake implementation of a datastore.
package datastore

import (
	"github.com/golang/protobuf/proto"
	"path"
	"strings"
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
	client_id string,
	do_not_lease bool) ([]*crypto_proto.GrrMessage, error) {
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
func (self *FakeDatastore) GetSubjectAttributes(
	config_obj *config.Config,
	urn string, attrs []string) (map[string][]byte, error) {

	result := make(map[string][]byte)
	data, pres := self.data[urn]
	if pres {
		for _, attr := range attrs {
			value, pres := data[attr]
			if pres {
				result[attr] = value
			}
		}
	}

	return result, nil
}
func (self *FakeDatastore) GetSubjectData(
	config_obj *config.Config,
	urn string,
	offset uint64,
	count uint64) (map[string][]byte, error) {
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

func (self *FakeDatastore) ListChildren(
	config_obj *config.Config,
	urn string,
	offset uint64, length uint64) ([]string, error) {
	var result []string

	data, err := self.GetSubjectData(config_obj, urn, offset, length)
	if err != nil {
		return nil, err
	}
	for predicate, _ := range data {
		if strings.HasPrefix(predicate, "index:") {
			result = append(result, path.Join(
				urn, strings.TrimPrefix(predicate, "index:dir/")))
		}
	}

	return result, nil
}

func init() {
	db := FakeDatastore{
		data: make(map[string]map[string][]byte),
	}

	RegisterImplementation("FakeDataStore", &db)
}
