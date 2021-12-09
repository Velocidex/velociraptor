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
	"os"
	"strings"
	"sync"
	"time"

	errors "github.com/pkg/errors"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	"www.velocidex.com/golang/velociraptor/file_store/api"
	"www.velocidex.com/golang/velociraptor/logging"
)

var (
	memcache_file_imp *MemcacheFileDataStore

	metricLRUHit = promauto.NewCounter(
		prometheus.CounterOpts{
			Name: "memcache_lru_hit",
			Help: "LRU for memcache",
		})

	metricLRUMiss = promauto.NewCounter(
		prometheus.CounterOpts{
			Name: "memcache_lru_miss",
			Help: "LRU for memcache",
		})

	metricIdleWriters = promauto.NewGauge(
		prometheus.GaugeOpts{
			Name: "memcache_idle_writers",
			Help: "Total available writers ready right now",
		})
)

const (
	MUTATION_OP_SET_SUBJECT = iota
	MUTATION_OP_DEL_SUBJECT
)

// Mark a mutation to be written to the backing data store.
type Mutation struct {
	op   int
	urn  api.DSPathSpec
	wg   *sync.WaitGroup
	data []byte

	// Will run when committed to disk.
	completion func()
}

type MemcacheFileDataStore struct {
	cache *MemcacheDatastore

	writer chan *Mutation
	ctx    context.Context
	cancel func()
}

func (self *MemcacheFileDataStore) invalidateDirCache(
	config_obj *config_proto.Config, urn api.DSPathSpec) {
	for len(urn.Components()) > 0 {
		path := urn.AsDatastoreDirectory(config_obj)
		self.cache.dir_cache.Remove(path)
		urn = urn.Dir()
	}
}

func (self *MemcacheFileDataStore) ExpirationPolicy(
	key string, value interface{}) bool {

	// Do not expire ping
	if strings.HasSuffix(key, "ping.db") {
		return false
	}

	return true
}

// Starts the writer loop.
func (self *MemcacheFileDataStore) StartWriter(
	ctx context.Context, wg *sync.WaitGroup,
	config_obj *config_proto.Config) {

	var timeout uint64
	var buffer_size, writers int

	if config_obj.Datastore != nil {
		timeout = config_obj.Datastore.MemcacheExpirationSec
		buffer_size = int(config_obj.Datastore.MemcacheWriteMutationBuffer)
		writers = int(config_obj.Datastore.MemcacheWriteMutationWriters)
	}
	if timeout == 0 {
		timeout = 600
	}
	self.cache.SetTimeout(time.Duration(timeout) * time.Second)
	self.cache.SetCheckExpirationCallback(self.ExpirationPolicy)

	if buffer_size <= 0 {
		buffer_size = 1000
	}
	self.writer = make(chan *Mutation, buffer_size)
	self.ctx = ctx

	if writers == 0 {
		writers = 100
	}

	// Start some writers.
	for i := 0; i < writers; i++ {
		metricIdleWriters.Inc()

		wg.Add(1)
		go func() {
			defer wg.Done()

			for {
				select {
				case <-ctx.Done():
					return

				case mutation, ok := <-self.writer:
					if !ok {
						return
					}

					metricIdleWriters.Dec()
					switch mutation.op {
					case MUTATION_OP_SET_SUBJECT:
						writeContentToFile(config_obj, mutation.urn, mutation.data)
						self.invalidateDirCache(config_obj, mutation.urn)

						// Call the completion function once we hit
						// the directory datastore.
						if mutation.completion != nil {
							mutation.completion()
						}

					case MUTATION_OP_DEL_SUBJECT:
						file_based_imp.DeleteSubject(config_obj, mutation.urn)
						self.invalidateDirCache(config_obj, mutation.urn.Dir())
					}

					metricIdleWriters.Inc()
					mutation.wg.Done()
				}
			}
		}()
	}
}

func (self *MemcacheFileDataStore) GetSubject(
	config_obj *config_proto.Config,
	urn api.DSPathSpec,
	message proto.Message) error {

	defer Instrument("read", "MemcacheFileDataStore", urn)()

	err := self.cache.GetSubject(config_obj, urn, message)
	if os.IsNotExist(errors.Cause(err)) {
		// The file is not in the cache, read it from the file system
		// instead.
		serialized_content, err := readContentFromFile(
			config_obj, urn, true /* must exist */)
		if err != nil {
			return err
		}

		metricLRUMiss.Inc()

		// Store it in the cache now
		self.cache.SetData(config_obj, urn, serialized_content)

		// This call should work because it is in cache.
		return self.cache.GetSubject(config_obj, urn, message)
	} else {
		metricLRUHit.Inc()
	}
	return err
}

func (self *MemcacheFileDataStore) SetSubjectWithCompletion(
	config_obj *config_proto.Config,
	urn api.DSPathSpec,
	message proto.Message,
	completion func()) error {

	defer Instrument("write", "MemcacheFileDataStore", urn)()

	// Encode as JSON
	var serialized_content []byte
	var err error

	if urn.Type() == api.PATH_TYPE_DATASTORE_JSON {
		serialized_content, err = protojson.Marshal(message)
		if err != nil {
			return err
		}

	} else {
		serialized_content, err = proto.Marshal(message)
		if err != nil {
			return err
		}
	}

	// Add the data to the cache.
	err = self.cache.SetSubject(config_obj, urn, message)

	// Send a SetSubject mutation to the writer loop.
	var wg sync.WaitGroup
	wg.Add(1)

	select {
	case <-self.ctx.Done():
		return nil

	case self.writer <- &Mutation{
		op:         MUTATION_OP_SET_SUBJECT,
		urn:        urn,
		wg:         &wg,
		completion: completion,
		data:       serialized_content}:
	}

	if config_obj.Datastore.MemcacheWriteMutationBuffer < 0 {
		wg.Wait()
	}

	return err
}

func (self *MemcacheFileDataStore) SetSubject(
	config_obj *config_proto.Config,
	urn api.DSPathSpec,
	message proto.Message) error {

	defer Instrument("write", "MemcacheFileDataStore", urn)()

	// Encode as JSON
	var serialized_content []byte
	var err error

	if urn.Type() == api.PATH_TYPE_DATASTORE_JSON {
		serialized_content, err = protojson.Marshal(message)
		if err != nil {
			return err
		}

	} else {
		serialized_content, err = proto.Marshal(message)
		if err != nil {
			return err
		}
	}

	// Add the data to the cache immediately.
	err = self.cache.SetSubject(config_obj, urn, message)

	if err != nil {
		return err
	}

	err = writeContentToFile(config_obj, urn, serialized_content)
	if err != nil {
		return err
	}

	self.invalidateDirCache(config_obj, urn)
	return err
}

func (self *MemcacheFileDataStore) DeleteSubject(
	config_obj *config_proto.Config,
	urn api.DSPathSpec) error {
	defer Instrument("delete", "MemcacheFileDataStore", urn)()

	err := self.cache.DeleteSubject(config_obj, urn)

	// Send a DeleteSubject mutation to the writer loop.
	var wg sync.WaitGroup
	wg.Add(1)

	select {
	case <-self.ctx.Done():
		break

	case self.writer <- &Mutation{
		op:  MUTATION_OP_DEL_SUBJECT,
		wg:  &wg,
		urn: urn}:
	}

	if config_obj.Datastore.MemcacheWriteMutationBuffer < 0 {
		wg.Wait()
	}

	return err
}

// Lists all the children of a URN.
func (self *MemcacheFileDataStore) ListChildren(
	config_obj *config_proto.Config,
	urn api.DSPathSpec) ([]api.DSPathSpec, error) {

	defer Instrument("list", "MemcacheFileDataStore", urn)()

	children, err := self.cache.ListChildren(config_obj, urn)
	if err != nil || len(children) == 0 {
		children, err = file_based_imp.ListChildren(config_obj, urn)
		if err != nil {
			return children, err
		}

		metricLRUMiss.Inc()

		// Store in the memcache.
		self.cache.SetChildren(config_obj, urn, children)

	} else {
		metricLRUHit.Inc()
	}
	return children, err
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

// Support RawDataStore interface
func (self *MemcacheFileDataStore) GetBuffer(
	config_obj *config_proto.Config,
	urn api.DSPathSpec) ([]byte, error) {

	bulk_data, err := self.cache.GetBuffer(config_obj, urn)
	if err == nil {
		metricLRUHit.Inc()
		return bulk_data, err
	}

	bulk_data, err = readContentFromFile(
		config_obj, urn, true /* must exist */)
	if err != nil {
		return nil, err
	}

	metricLRUMiss.Inc()
	self.cache.SetData(config_obj, urn, bulk_data)

	return bulk_data, nil
}

// Needed to support RawDataStore interface.
func (self *MemcacheFileDataStore) SetBuffer(
	config_obj *config_proto.Config,
	urn api.DSPathSpec, data []byte, completion func()) error {

	err := self.cache.SetData(config_obj, urn, data)
	if err != nil {
		return err
	}

	var wg sync.WaitGroup
	wg.Add(1)
	select {
	case <-self.ctx.Done():
		return nil

	case self.writer <- &Mutation{
		op:         MUTATION_OP_SET_SUBJECT,
		urn:        urn,
		wg:         &wg,
		data:       data,
		completion: completion,
	}:
	}

	if config_obj.Datastore.MemcacheWriteMutationBuffer < 0 {
		wg.Wait()
	}
	return nil
}

// Recursively makes sure the directories are added to the cache. We
// treat the file backing as authoritative, so if the dir cache is not
// present in cache we read intermediate paths from disk.
func get_file_dir_metadata(
	dir_cache *DirectoryLRUCache,
	config_obj *config_proto.Config, urn api.DSPathSpec) (
	*DirectoryMetadata, error) {

	// Check if the top level directory contains metadata.
	path := urn.AsDatastoreDirectory(config_obj)

	// Fast path - the directory exists in the cache. NOTE: We dont
	// need to maintain the directories on the filesystem as the
	// FileBaseDataStore already does this.
	md, pres := dir_cache.Get(path)
	if pres {
		return md, nil
	}

	// DirectoryMetadata is not known, fetch the directory listing
	// from the filesystem
	children, err := file_based_imp.ListChildren(config_obj, urn)
	if err == nil {
		md = NewDirectoryMetadata()
		for _, child := range children {
			key := child.Base() + api.GetExtensionForDatastore(child)
			md.Set(key, child)
		}
		dir_cache.Set(path, md)
	}

	// Cache this for next time.
	dir_cache.Set(path, md)
	return md, nil
}

func NewMemcacheFileDataStore() *MemcacheFileDataStore {
	result := &MemcacheFileDataStore{
		cache: NewMemcacheDataStore(),
	}
	return result
}

func StartMemcacheFileService(
	ctx context.Context, wg *sync.WaitGroup,
	config_obj *config_proto.Config) error {

	if memcache_file_imp != nil {
		logger := logging.GetLogger(config_obj, &logging.FrontendComponent)
		logger.Info("<green>Starting</> memcache service")
		memcache_file_imp.StartWriter(ctx, wg, config_obj)
	}

	return nil
}
