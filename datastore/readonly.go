// Read only memory datastore implementation - read from directory but
// writes stay in memory.

package datastore

import (
	"context"

	"google.golang.org/protobuf/proto"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	"www.velocidex.com/golang/velociraptor/file_store/api"
	"www.velocidex.com/golang/velociraptor/utils"
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
	if completion != nil &&
		!utils.CompareFuncs(completion, utils.SyncCompleter) {
		completion()
	}
	return err
}

func (self *ReadOnlyDataStore) DeleteSubject(
	config_obj *config_proto.Config,
	urn api.DSPathSpec) error {
	return self.cache.DeleteSubject(config_obj, urn)
}

func NewReadOnlyDataStore(
	ctx context.Context,
	config_obj *config_proto.Config) *ReadOnlyDataStore {
	return &ReadOnlyDataStore{&MemcacheFileDataStore{
		cache: NewMemcacheDataStore(ctx, config_obj),
	}}
}
