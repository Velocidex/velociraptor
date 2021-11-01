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
	memcache_file_imp = NewMemcacheFileDataStore()

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
	ctx    context.Context
	cancel func()
}

// Starts the writer loop.
func (self *MemcacheFileDataStore) StartWriter(
	ctx context.Context, wg *sync.WaitGroup,
	config_obj *config_proto.Config) {

	var timeout uint64
	var buffer_size int

	if config_obj.Datastore != nil {
		timeout = config_obj.Datastore.MemcacheExpirationSec
		buffer_size = int(config_obj.Datastore.MemcacheWriteMutationBuffer)
	}
	if timeout == 0 {
		timeout = 600
	}
	self.cache.SetTimeout(time.Duration(timeout) * time.Second)

	if buffer_size == 0 {
		buffer_size = 1000
	}
	self.writer = make(chan Mutation, buffer_size)
	self.ctx = ctx

	// Start some writers.
	for i := 0; i < 5; i++ {
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
}

func (self *MemcacheFileDataStore) GetSubject(
	config_obj *config_proto.Config,
	urn api.DSPathSpec,
	message proto.Message) error {

	defer Instrument("read", urn)()

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

func (self *MemcacheFileDataStore) SetSubject(
	config_obj *config_proto.Config,
	urn api.DSPathSpec,
	message proto.Message) error {

	defer Instrument("write", urn)()

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
	select {
	case <-self.ctx.Done():
		return nil

	case self.writer <- Mutation{
		op:   MUTATION_OP_SET_SUBJECT,
		urn:  urn,
		data: serialized_content}:
	}

	return err
}

func (self *MemcacheFileDataStore) DeleteSubject(
	config_obj *config_proto.Config,
	urn api.DSPathSpec) error {
	defer Instrument("delete", urn)()

	err := self.cache.DeleteSubject(config_obj, urn)

	// Send a DeleteSubject mutation to the writer loop.
	select {
	case <-self.ctx.Done():
		break

	case self.writer <- Mutation{
		op:  MUTATION_OP_DEL_SUBJECT,
		urn: urn}:
	}

	return err
}

// Lists all the children of a URN.
func (self *MemcacheFileDataStore) ListChildren(
	config_obj *config_proto.Config,
	urn api.DSPathSpec) ([]api.DSPathSpec, error) {

	defer Instrument("list", urn)()
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

func (self *MemcacheFileDataStore) Walk(config_obj *config_proto.Config,
	root api.DSPathSpec, walkFn WalkFunc) error {

	all_children, err := self.ListChildren(config_obj, root)
	if err != nil {
		return err
	}

	for _, child := range all_children {
		// Recurse into directories
		if child.IsDir() {
			err := self.Walk(config_obj, child, walkFn)
			if err != nil {
				// Do not quit the walk early.
			}
		} else {
			err := walkFn(child)
			if err == StopIteration {
				return nil
			}
			continue
		}
	}

	return nil
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

// Recursively makes sure the directories are added to the cache. We
// treat the file backing as authoritative, so if the dir cache is not
// present in cache we read intermediate paths from disk.
func file_based_mkdirall(
	dir_cache *DirectoryLRUCache,
	config_obj *config_proto.Config, urn api.DSPathSpec) (
	*DirectoryMetadata, error) {

	// Check if the top level directory contains metadata.
	path := urn.AsDatastoreDirectory(config_obj)

	// Fast path - the directory exists in the cache.
	md, pres := dir_cache.Get(path)
	if pres {
		return md, nil
	}

	// Fetch the directory listing from the filesystem
	children, err := file_based_imp.ListChildren(config_obj, urn)
	if err == nil {
		md = NewDirectoryMetadata()
		for _, child := range children {
			md.Set(child.Base(), child)
		}
		dir_cache.Set(path, md)
	}

	// Create top level and every level under it.
	dir_cache.Set(path, md)

	for len(urn.Components()) > 0 {
		parent := urn.Dir()
		path := parent.AsDatastoreDirectory(config_obj)

		// Retrace all parent directories - at some point we will hit
		// a cache directory and stop making filesystem calls.
		intermediate_md, pres := dir_cache.Get(path)
		if !pres {
			// Fetch the directory listing from the filesystem
			children, err := file_based_imp.ListChildren(config_obj, parent)
			if err == nil {
				intermediate_md = NewDirectoryMetadata()
				for _, child := range children {
					intermediate_md.Set(child.Base(), child)
				}
				dir_cache.Set(path, intermediate_md)
			}
		}

		// Make sure the current directory contains the current
		// component. If it does not we need to add it as the new file
		// will create the intermediate directory.
		// If the intermediate directory already exists we can exit early.
		base := urn.Base()
		_, pres = intermediate_md.Get(base)
		if pres {
			return md, nil
		}
		intermediate_md.Set(urn.Base(), urn)

		urn = parent
	}

	return md, nil
}

func NewMemcacheFileDataStore() *MemcacheFileDataStore {
	result := &MemcacheFileDataStore{
		cache: NewMemcacheDataStore(),
	}

	result.cache.SetMkDirAll(file_based_mkdirall)
	return result
}

func StartMemcacheFileService(
	ctx context.Context, wg *sync.WaitGroup,
	config_obj *config_proto.Config) error {
	if config_obj.Datastore != nil &&
		config_obj.Datastore.Implementation == "MemcacheFileDataStore" {
		logger := logging.GetLogger(config_obj, &logging.FrontendComponent)
		logger.Info("<green>Starting</> memcache service")
		memcache_file_imp.StartWriter(ctx, wg, config_obj)
	}

	return nil
}
