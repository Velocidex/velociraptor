// This implements a memory cached datastore with asynchronous file
// backing:

// * Writes are cached immediately into memory and a write mutation is
//   sent to the writer channel.
// * A writer loop writes the mutations into the underlying file
//   backed store.
// * Reads are obtained from the memcache if possible, otherwise they
//   fall through to the file backed data store.

package datastore

import (
	"context"
	"sync"

	"github.com/ReneKroon/ttlcache/v2"
	"google.golang.org/protobuf/proto"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	"www.velocidex.com/golang/velociraptor/file_store/api"
)

var (
	memcache_file_imp = NewMemcacheFileDataStore()
)

const (
	MUTATION_OP_SET_SUBJECT = iota
	MUTATION_OP_DEL_SUBJECT
)

// Mark a mutation to be written to the backing data store.
type Mutation struct {
	op   int
	urn  api.DSPathSpec
	data []byte
}

type MemcacheFileDataStore struct {
	cache *MemcacheDatastore

	writer chan Mutation
}

// Starts the writer loop.
func (self *MemcacheFileDataStore) StartWriter(
	ctx context.Context, wg *sync.WaitGroup,
	config_obj *config_proto.Config) {

	self.writer = make(chan Mutation)

	wg.Add(1)
	go func() {
		defer wg.Done()
		defer close(self.writer)

		for {
			select {
			case <-ctx.Done():
				return

			case mutation, ok := <-self.writer:
				if !ok {
					return
				}

				switch mutation.op {
				case MUTATION_OP_SET_SUBJECT:
					writeContentToFile(config_obj, mutation.urn, mutation.data)
				case MUTATION_OP_DEL_SUBJECT:
					file_based_imp.DeleteSubject(config_obj, mutation.urn)
				}
			}
		}
	}()
}

func (self *MemcacheFileDataStore) GetSubject(
	config_obj *config_proto.Config,
	urn api.DSPathSpec,
	message proto.Message) error {

	defer Instrument("read", urn)()
	return self.cache.GetSubject(config_obj, urn, message)
}

func (self *MemcacheFileDataStore) SetSubject(
	config_obj *config_proto.Config,
	urn api.DSPathSpec,
	message proto.Message) error {

	defer Instrument("write", urn)()
	return self.cache.SetSubject(config_obj, urn, message)
}

func (self *MemcacheFileDataStore) DeleteSubject(
	config_obj *config_proto.Config,
	urn api.DSPathSpec) error {
	defer Instrument("delete", urn)()

	return self.cache.DeleteSubject(config_obj, urn)
}

// Lists all the children of a URN.
func (self *MemcacheFileDataStore) ListChildren(
	config_obj *config_proto.Config,
	urn api.DSPathSpec) ([]api.DSPathSpec, error) {

	defer Instrument("list", urn)()
	return self.cache.ListChildren(config_obj, urn)
}

func (self *MemcacheFileDataStore) Walk(config_obj *config_proto.Config,
	root api.DSPathSpec, walkFn WalkFunc) error {
	return self.cache.Walk(config_obj, root, walkFn)
}

func (self *MemcacheFileDataStore) Close() {
	self.cache.Close()
}

func (self *MemcacheFileDataStore) Clear() {
	self.cache.Clear()
}

func (self *MemcacheFileDataStore) Debug(config_obj *config_proto.Config) {
	self.cache.Debug(config_obj)
}

func (self *MemcacheFileDataStore) Dump() []api.DSPathSpec {
	return self.cache.Dump()
}

func NewMemcacheFileDataStore() *MemcacheFileDataStore {
	return &MemcacheFileDataStore{
		cache: &MemcacheDatastore{
			data_cache: ttlcache.NewCache(),
			dir_cache:  ttlcache.NewCache(),
		}}
}
