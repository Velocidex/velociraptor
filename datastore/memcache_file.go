// This implements a memory cached datastore with asynchronous file
// backing:

// * Writes are cached immediately into memory and a write mutation is
//   sent to the writer channel.
// * A writer loop writes the mutations into the underlying file
//   backed store.
// * Reads are obtained from the memcache if possible, otherwise they
//   fall through to the file backed data store.

/*
  ## A note about cache coherency for directory cache.

  The directory cache stores an in memory list of paths that belong
  inside a directory: Key: Datastore path -> Value: DirectoryMetadata

  The directory cache is designed to service ListChildren() calls.

  The filesystem is the ultimate source of truth for the cache.

  1. ListChildren of an uncached directory: Deledate to the
     FileBaseDataStore and cache the results.

  2. SetData of a data file (e.g. /a/b/c.json.db):

     * Find the containing directory (/a/b) and read the
       DirectoryMetadata. If DirectoryMetadata is not cached fetch
       from disk.

     * If present, we set a new member (c.json.db) in it.

     * Walk the tree back to adjust parent directories - here we have
       to be careful to not read the filesystem unnecessarily so we
       just invalidate every directory cache :

        - If a parent DirectoryMetadata exists, we directly add
          member.

        - If there is not intermediate memory cache, then exit.
*/

package datastore

import (
	"context"
	"errors"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	"www.velocidex.com/golang/velociraptor/file_store/api"
	"www.velocidex.com/golang/velociraptor/logging"
	"www.velocidex.com/golang/velociraptor/utils"
)

var (
	memcache_file_imp *MemcacheFileDataStore

	notInitializedError = errors.New("MemcacheFileDataStore not initialized!")

	metricDirLRUHit = promauto.NewCounter(
		prometheus.CounterOpts{
			Name: "memcache_lru_dir_hit",
			Help: "LRU for memcache",
		})

	metricDirLRUMiss = promauto.NewCounter(
		prometheus.CounterOpts{
			Name: "memcache_lru_dir_miss",
			Help: "LRU for memcache",
		})

	metricDataLRUHit = promauto.NewCounter(
		prometheus.CounterOpts{
			Name: "memcache_lru_data_hit",
			Help: "LRU for memcache",
		})

	metricDataLRUMiss = promauto.NewCounter(
		prometheus.CounterOpts{
			Name: "memcache_lru_data_miss",
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
	op  int
	urn api.DSPathSpec

	// The config object related to the org this call came from
	org_config_obj *config_proto.Config
	wg             *sync.WaitGroup
	data           []byte

	// Will run when committed to disk.
	completion func()
}

type MemcacheFileDataStore struct {
	mu    sync.Mutex
	cache *MemcacheDatastore

	writer chan *Mutation
	ctx    context.Context

	started bool
}

func (self *MemcacheFileDataStore) Healthy() error {
	return nil
}

func (self *MemcacheFileDataStore) Stats() *MemcacheStats {
	return self.cache.Stats()
}

func (self *MemcacheFileDataStore) invalidateDirCache(
	config_obj *config_proto.Config, urn api.DSPathSpec) {

	for len(urn.Components()) > 0 {
		path := AsDatastoreDirectory(self, config_obj, urn)
		md, pres := self.cache.dir_cache.Get(path)
		if pres && !md.IsFull() {
			key_path := AsDatastoreDirectory(self, config_obj, urn)
			_ = self.cache.dir_cache.Remove(key_path)
		}
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

func (self *MemcacheFileDataStore) Flush() {
	for {
		select {
		case mutation, ok := <-self.writer:
			if !ok {
				return
			}
			self.processMutation(mutation)
		default:
			return
		}
	}
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

	if buffer_size < 0 {
		buffer_size = 1000
	}
	self.mu.Lock()
	self.writer = make(chan *Mutation, buffer_size)
	self.ctx = ctx
	self.started = true
	self.mu.Unlock()

	if writers == 0 {
		writers = 100
	}

	// Start some writers.
	for i := 0; i < writers; i++ {
		metricIdleWriters.Inc()

		wg.Add(1)
		go func() {
			defer wg.Done()

			self.mu.Lock()
			writer := self.writer
			self.mu.Unlock()

			for {
				select {
				case <-ctx.Done():
					return

				case mutation, ok := <-writer:
					if !ok {
						return
					}
					self.processMutation(mutation)
				}
			}
		}()
	}
}

func (self *MemcacheFileDataStore) processMutation(mutation *Mutation) {
	metricIdleWriters.Dec()
	switch mutation.op {
	case MUTATION_OP_SET_SUBJECT:
		err := writeContentToFile(self, mutation.org_config_obj, mutation.urn, mutation.data)
		if err != nil {
			logger := logging.GetLogger(mutation.org_config_obj, &logging.FrontendComponent)
			logger.Error("MemcacheFileDataStore: processMutation: %v", err)
		}
		self.invalidateDirCache(mutation.org_config_obj, mutation.urn)

		// Call the completion function once we hit
		// the directory datastore.
		if mutation.completion != nil {
			mutation.completion()
		}

	case MUTATION_OP_DEL_SUBJECT:
		err := file_based_imp.DeleteSubject(mutation.org_config_obj, mutation.urn)
		if err != nil {
			logger := logging.GetLogger(mutation.org_config_obj, &logging.FrontendComponent)
			logger.Error("MemcacheFileDataStore: processMutation: %v", err)
		}

		self.invalidateDirCache(mutation.org_config_obj, mutation.urn.Dir())

		// Call the completion function once we hit
		// the directory datastore.
		if mutation.completion != nil {
			mutation.completion()
		}
	}

	metricIdleWriters.Inc()
	if mutation.wg != nil {
		mutation.wg.Done()
	}
}

func (self *MemcacheFileDataStore) GetSubject(
	config_obj *config_proto.Config,
	urn api.DSPathSpec,
	message proto.Message) error {

	self.mu.Lock()
	defer self.mu.Unlock()

	defer Instrument("read", "MemcacheFileDataStore", urn)()

	err := self.cache.GetSubject(config_obj, urn, message)
	if errors.Is(err, os.ErrNotExist) {
		// The file is not in the cache, read it from the file system
		// instead.
		serialized_content, err := readContentFromFile(self, config_obj, urn)
		if err != nil {
			return err
		}

		metricDataLRUMiss.Inc()

		// Store it in the cache for next time.
		err = self.cache.SetData(config_obj, urn, serialized_content)
		if err != nil {
			return err
		}

		// Unmarshal the data into the message.
		return unmarshalData(serialized_content, urn, message)
	} else {
		metricDataLRUHit.Inc()
	}
	return err
}

func (self *MemcacheFileDataStore) maybeComplete(c func()) {
	if c != nil {
		c()
	}
}

func (self *MemcacheFileDataStore) SetSubjectWithCompletion(
	config_obj *config_proto.Config,
	urn api.DSPathSpec,
	message proto.Message,
	completion func()) error {

	// MemcacheFileDataStore is asynchronous: Only complete on errors,
	// but pass completion function to the writer pool.

	defer Instrument("write", "MemcacheFileDataStore", urn)()

	// Encode as JSON
	var serialized_content []byte
	var err error

	if urn.Type() == api.PATH_TYPE_DATASTORE_JSON {
		serialized_content, err = protojson.Marshal(message)
		if err != nil {
			self.maybeComplete(completion)
			return err
		}

	} else {
		serialized_content, err = proto.Marshal(message)
		if err != nil {
			self.maybeComplete(completion)
			return err
		}
	}

	// Add the data to the memory cache (do not call completion yet
	// until we sync the file based datastore).
	err = self.cache.SetSubjectWithCompletion(config_obj, urn, message, nil)

	if self.ctx == nil {
		return notInitializedError
	}

	// Send a SetSubject mutation to the writer loop.
	var wg sync.WaitGroup
	mutation := &Mutation{
		op:             MUTATION_OP_SET_SUBJECT,
		urn:            urn,
		org_config_obj: config_obj,
		wg:             &wg,
		completion:     completion,
		data:           serialized_content}

	// If the call is synchronous we need to wait here until it is
	// flushed to disk.
	if utils.CompareFuncs(mutation.completion, utils.SyncCompleter) {
		wg.Add(1)
		defer wg.Wait()
		mutation.completion = wg.Done
	}

	wg.Add(1)
	select {
	case <-self.ctx.Done():
		// If we exit this function we need to call the completion,
		// otherwise let the writer call the completion.
		self.maybeComplete(completion)
		return nil

		// After we send this to the channel, the writer will
		// complete.
	case self.writer <- mutation:
	}

	// Config file switches off asynchronous writes, wait here for
	// completion.
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
	err = self.cache.SetData(config_obj, urn, serialized_content)
	if err != nil {
		return err
	}

	err = writeContentToFile(self, config_obj, urn, serialized_content)
	if err != nil {
		return err
	}

	return err
}

func (self *MemcacheFileDataStore) DeleteSubject(
	config_obj *config_proto.Config,
	urn api.DSPathSpec) error {
	defer Instrument("delete", "MemcacheFileDataStore", urn)()

	if self.ctx == nil {
		return notInitializedError
	}

	// Remove immediately from the cache memcache as soon as the file
	// is removed from disk.
	completion := func() {
		_ = self.cache.DeleteSubject(config_obj, urn)
	}

	// Send a DeleteSubject mutation to the writer loop.
	wg := &sync.WaitGroup{}
	wg.Add(1)

	select {
	case <-self.ctx.Done():
		completion()
		break

	case self.writer <- &Mutation{
		op: MUTATION_OP_DEL_SUBJECT,
		wg: wg,

		// When we complete make sure the cache is also invalidated to
		// avoid racing with GetSubject().
		completion:     completion,
		urn:            urn,
		org_config_obj: config_obj}:
	}

	if config_obj.Datastore.MemcacheWriteMutationBuffer < 0 {
		wg.Wait()
	}

	return nil
}

func (self *MemcacheFileDataStore) DeleteSubjectWithCompletion(
	config_obj *config_proto.Config,
	urn api.DSPathSpec, completion func()) error {
	defer Instrument("delete", "MemcacheFileDataStore", urn)()

	mutation := &Mutation{
		op:             MUTATION_OP_DEL_SUBJECT,
		urn:            urn,
		completion:     completion,
		org_config_obj: config_obj,
	}

	// Delete inline - wait for the operation to complete before returning.
	if utils.CompareFuncs(completion, utils.SyncCompleter) {
		mutation.completion = nil
		self.processMutation(mutation)
		return self.cache.DeleteSubjectWithCompletion(config_obj, urn, completion)
	}

	// Remove immediately from the cache memcache as soon as the file
	// is removed from disk.
	mutation.completion = func() {
		_ = self.cache.DeleteSubject(config_obj, urn)
		if completion != nil {
			completion()
		}
	}

	if self.ctx == nil {
		return notInitializedError
	}

	select {
	case <-self.ctx.Done():
		if completion != nil {
			completion()
		}
		break

	case self.writer <- mutation:
	}

	return nil
}

// Lists all the children of a URN.
func (self *MemcacheFileDataStore) ListChildren(
	config_obj *config_proto.Config,
	urn api.DSPathSpec) ([]api.DSPathSpec, error) {

	// No locking here!  This function encompases the fast memcache
	// **and** the slow filesystem. Locking here will deadlock on the
	// slow filesystem.

	defer Instrument("list", "MemcacheFileDataStore", urn)()

	children, err := self.cache.ListChildren(config_obj, urn)
	if err != nil || children == nil {
		children, err = file_based_imp.ListChildren(config_obj, urn)
		if err != nil {
			return children, err
		}

		metricDirLRUMiss.Inc()

		// Store in the memcache.
		self.cache.SetChildren(config_obj, urn, children)

	} else {
		metricDirLRUHit.Inc()
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
		metricDataLRUHit.Inc()
		return bulk_data, err
	}

	bulk_data, err = readContentFromFile(self, config_obj, urn)
	if err != nil {
		return nil, err
	}

	metricDataLRUMiss.Inc()
	err = self.cache.SetData(config_obj, urn, bulk_data)

	return bulk_data, err
}

// Needed to support RawDataStore interface.
func (self *MemcacheFileDataStore) SetBuffer(
	config_obj *config_proto.Config,
	urn api.DSPathSpec, data []byte, completion func()) error {

	err := self.cache.SetData(config_obj, urn, data)
	if err != nil {
		return err
	}

	if self.ctx == nil {
		return notInitializedError
	}

	var wg sync.WaitGroup
	wg.Add(1)
	select {
	case <-self.ctx.Done():
		return nil

	case self.writer <- &Mutation{
		op:             MUTATION_OP_SET_SUBJECT,
		urn:            urn,
		org_config_obj: config_obj,
		wg:             &wg,
		data:           data,
		completion:     completion,
	}:
	}

	if config_obj.Datastore.MemcacheWriteMutationBuffer < 0 {
		wg.Wait()
	}
	return nil
}

// Recursively makes sure the directories are added to the cache.
func get_file_dir_metadata(
	dir_cache *DirectoryLRUCache,
	db DataStore, config_obj *config_proto.Config, urn api.DSPathSpec) (
	*DirectoryMetadata, error) {

	// Check if the top level directory contains metadata.
	path := AsDatastoreDirectory(db, config_obj, urn)

	// Fast path - the directory exists in the cache. NOTE: We dont
	// need to maintain the directories on the filesystem as the
	// FileBaseDataStore already does this. If DirectoryMetadata
	// exists in the cache then it must reflect the current state of
	// the filesystem.
	md, pres := dir_cache.Get(path)
	if pres {
		return md, nil
	}

	// We have no cached metadata object. We can create one but this
	// will just cause more filesystem activity because we dont know
	// what files exist in order to construct a new DirectoryMetadata.
	// Since DirectoryMetadata caches are only used for ListChildren()
	// calls, there is no point us filling the metadata in advance of
	// a ListChildren() because that may not be required.

	// So the most logical thing to do here is to just not have a
	// DirectoryMetadata at all - future calls for ListChildren() will
	// perform a filesystem op and fill in the cache if needed.
	urn = urn.Dir()
	for len(urn.Components()) > 0 {
		path := AsDatastoreDirectory(db, config_obj, urn)
		md, pres := dir_cache.Get(path)
		if pres && !md.IsFull() {
			key_path := AsDatastoreDirectory(db, config_obj, urn)
			err := dir_cache.Remove(key_path)
			if err != nil {
				return nil, err
			}
		}
		urn = urn.Dir()
	}

	return nil, errorNoDirectoryMetadata
}

func NewMemcacheFileDataStore(
	ctx context.Context,
	config_obj *config_proto.Config) *MemcacheFileDataStore {
	data_max_size := 10000
	if config_obj.Datastore != nil &&
		config_obj.Datastore.MemcacheDatastoreMaxSize > 0 {
		data_max_size = int(config_obj.Datastore.MemcacheDatastoreMaxSize)
	}

	data_max_item_size := 64 * 1024
	if config_obj.Datastore.MemcacheDatastoreMaxItemSize > 0 {
		data_max_item_size = int(config_obj.Datastore.MemcacheDatastoreMaxItemSize)
	}

	dir_max_item_size := 1000
	if config_obj.Datastore.MemcacheDatastoreMaxDirSize > 0 {
		dir_max_item_size = int(config_obj.Datastore.MemcacheDatastoreMaxDirSize)
	}

	result := &MemcacheFileDataStore{
		cache: &MemcacheDatastore{
			data_cache: NewDataLRUCache(ctx, config_obj,
				data_max_size, data_max_item_size),
			dir_cache: NewDirectoryLRUCache(ctx, config_obj,
				data_max_size, dir_max_item_size),
			get_dir_metadata: get_file_dir_metadata,
		},
	}

	return result
}

func StartMemcacheFileService(
	ctx context.Context, wg *sync.WaitGroup,
	config_obj *config_proto.Config) error {

	if config_obj.Datastore == nil {
		return nil
	}

	db, err := GetDB(config_obj)
	if err != nil {
		return err
	}

	memcache_file_db, ok := db.(*MemcacheFileDataStore)
	if !ok {
		// If it not a MemcacheFileDataStore so we dont need to do
		// anything to it.
		return nil
	}

	if !memcache_file_db.started {
		logger := logging.GetLogger(config_obj, &logging.FrontendComponent)
		logger.Info("<green>Starting</> memcache service")
		memcache_file_db.StartWriter(ctx, wg, config_obj)
	}

	return nil
}
