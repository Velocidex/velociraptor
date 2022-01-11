// Read only memory datastore implementation - read from directory but
// writes stay in memory.

package datastore

import (
	"google.golang.org/protobuf/proto"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	"www.velocidex.com/golang/velociraptor/file_store/api"
)

var (
	read_only_imp *ReadOnlyDataStore
)

type ReadOnlyDataStore struct {
	*MemcacheFileDataStore
}

func (self *ReadOnlyDataStore) SetSubject(
	config_obj *config_proto.Config,
	urn api.DSPathSpec,
	message proto.Message) error {

	// Add the data to the cache immediately.
	err := self.cache.SetSubject(config_obj, urn, message)
	return err
}

func (self *ReadOnlyDataStore) SetSubjectWithCompletion(
	config_obj *config_proto.Config,
	urn api.DSPathSpec,
	message proto.Message,
	completion func()) error {

	err := self.cache.SetSubject(config_obj, urn, message)
	if completion != nil {
		completion()
	}
	return err
}

func (self *ReadOnlyDataStore) DeleteSubject(
	config_obj *config_proto.Config,
	urn api.DSPathSpec) error {
	return self.cache.DeleteSubject(config_obj, urn)
}

func NewReadOnlyDataStore(config_obj *config_proto.Config) *ReadOnlyDataStore {
	return &ReadOnlyDataStore{&MemcacheFileDataStore{
		cache: NewMemcacheDataStore(config_obj),
	}}
}
