package datastore

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/prometheus/client_golang/prometheus"

	"github.com/Velocidex/ttlcache/v2"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	"www.velocidex.com/golang/velociraptor/file_store/api"
	"www.velocidex.com/golang/velociraptor/file_store/path_specs"
	"www.velocidex.com/golang/velociraptor/utils"
)

var (
	memcache_imp DataStore

	internalError            = errors.New("Internal datastore error")
	errorOversize            = errors.New("Oversize")
	errorNoDirectoryMetadata = errors.New("No Directory Metadata")
)

// Prometheus panics with multiple registrations which trigger on
// tests.
func registerGauge(g prometheus.Collector) {
	prometheus.Unregister(g)
	_ = prometheus.Register(g)
}

func RegisterMemcacheDatastoreMetrics(db MemcacheStater) {
	// These might return an error if they are called more than once,
	// but we assume under normal operation the config_obj does not
	// change, therefore the datastore does not really change. So it
	// is ok to ignore these errors.
	registerGauge(prometheus.NewGaugeFunc(
		prometheus.GaugeOpts{
			Name: "memcache_dir_lru_total",
			Help: "Total directories cached",
		}, func() float64 {
			stats := db.Stats()
			return float64(stats.DirItemCount)
		}))

	registerGauge(prometheus.NewGaugeFunc(
		prometheus.GaugeOpts{
			Name: "memcache_data_lru_total",
			Help: "Total files cached",
		}, func() float64 {
			stats := db.Stats()
			return float64(stats.DataItemCount)
		}))

	registerGauge(prometheus.NewGaugeFunc(
		prometheus.GaugeOpts{
			Name: "memcache_dir_lru_total_bytes",
			Help: "Total directories cached",
		}, func() float64 {
			stats := db.Stats()
			return float64(stats.DirItemSize)
		}))

	registerGauge(prometheus.NewGaugeFunc(
		prometheus.GaugeOpts{
			Name: "memcache_data_lru_total_bytes",
			Help: "Total bytes cached",
		}, func() float64 {
			stats := db.Stats()
			return float64(stats.DataItemSize)
		}))
}

// Stored in data_cache contains bulk data.
type BulkData struct {
	mu   sync.Mutex
	data []byte
}

func (self *BulkData) Len() int {
	self.mu.Lock()
	defer self.mu.Unlock()

	return len(self.data)
}

// Stored in dir_cache - contains information about a directory.
//   - List of children in the directory.
//   - If the directory is too large we cache this information: Further
//     listings will delegate to file based datastore.
type DirectoryMetadata struct {
	mu   sync.Mutex
	data map[string]api.DSPathSpec

	max_size int

	// If this is set we know that the directory is too large to cache
	// in memory. The data map above will be cleared.
	full bool
}

func (self *DirectoryMetadata) IsFull() bool {
	self.mu.Lock()
	defer self.mu.Unlock()

	return self.full
}

func (self *DirectoryMetadata) Debug() string {
	self.mu.Lock()
	defer self.mu.Unlock()

	first_item := ""
	for k := range self.data {
		first_item = k
		break
	}

	return fmt.Sprintf("DirectoryMetadata len %d (%v)",
		len(self.data), first_item)
}

// An indication of how many bytes the entry is taking - for now use
// the length of the path as a proxy for the full size so we dont need
// to calculate too much.
func (self *DirectoryMetadata) Bytes() int {
	self.mu.Lock()
	defer self.mu.Unlock()

	size := 0
	for k := range self.data {
		size += len(k)
	}

	return size
}

// Update the DirectoryMetadata with a new child.
func (self *DirectoryMetadata) Set(urn api.DSPathSpec) {
	self.mu.Lock()
	defer self.mu.Unlock()

	// If we are full, next read will come from disk anyway so we dont
	// bother.
	if self.full {
		return
	}

	key := urn.Base() + api.GetExtensionForDatastore(urn)
	self.data[key] = urn

	// Too many files to cache, we stop caching any more but mark this
	// directory as full.
	if len(self.data) > self.max_size {
		self.data = make(map[string]api.DSPathSpec)
		self.full = true
	}
}

func (self *DirectoryMetadata) Remove(urn api.DSPathSpec) {
	self.mu.Lock()
	defer self.mu.Unlock()

	key := urn.Base() + api.GetExtensionForDatastore(urn)
	delete(self.data, key)
}

func (self *DirectoryMetadata) Len() int {
	self.mu.Lock()
	defer self.mu.Unlock()

	return len(self.data)
}

func (self *DirectoryMetadata) Items() []api.DSPathSpec {
	self.mu.Lock()
	defer self.mu.Unlock()

	result := make([]api.DSPathSpec, 0, len(self.data))
	for _, i := range self.data {
		result = append(result, i)
	}

	sort.Slice(result, func(i, j int) bool {
		return result[i].Base() < result[j].Base()
	})
	return result
}

func (self *DirectoryMetadata) Get(key string) (api.DSPathSpec, bool) {
	self.mu.Lock()
	defer self.mu.Unlock()

	value, pres := self.data[key]
	return value, pres
}

func NewDirectoryMetadata(max_size int) *DirectoryMetadata {
	return &DirectoryMetadata{
		data:     make(map[string]api.DSPathSpec),
		max_size: max_size,
	}
}

type DirectoryLRUCache struct {
	mu sync.Mutex

	*ttlcache.Cache
	max_item_size int
}

func (self *DirectoryLRUCache) Get(path string) (*DirectoryMetadata, bool) {
	md_any, err := self.Cache.Get(path)
	if err != nil {
		return nil, false
	}

	md, ok := md_any.(*DirectoryMetadata)
	if !ok {
		return nil, false
	}
	return md, true
}

func (self *DirectoryLRUCache) Set(
	key_path string, value *DirectoryMetadata) error {
	self.mu.Lock()
	defer self.mu.Unlock()

	return self.Cache.Set(key_path, value)
}

// The size of the LRU is to total size
func (self *DirectoryLRUCache) Size() int {
	self.mu.Lock()
	defer self.mu.Unlock()

	size := 0
	for _, key := range self.GetKeys() {
		md, pres := self.Get(key)
		if pres {
			size += md.Bytes()
		}
	}
	return size
}

func (self *DirectoryLRUCache) NewDirectoryMetadata(path string) *DirectoryMetadata {
	md := &DirectoryMetadata{
		data:     make(map[string]api.DSPathSpec),
		max_size: self.max_item_size,
	}
	_ = self.Set(path, md)
	return md
}

func (self *DirectoryLRUCache) Count() int {
	self.mu.Lock()
	defer self.mu.Unlock()

	return len(self.Cache.GetKeys())
}

func NewDirectoryLRUCache(
	ctx context.Context, config_obj *config_proto.Config,
	max_size, max_item_size int) *DirectoryLRUCache {

	result := &DirectoryLRUCache{
		Cache: ttlcache.NewCache(),

		// Maximum length of directories we will cache (count of
		// children).
		max_item_size: max_item_size,
	}

	go func() {
		<-ctx.Done()
		result.Cache.Close()
	}()

	result.Cache.SetCacheSizeLimit(max_size)
	return result
}

// This is a memory cached data store.
type MemcacheDatastore struct {
	// Stores data like key value
	data_cache *DataLRUCache

	// Stores directory metadata.
	dir_cache *DirectoryLRUCache

	// Gets the relevant DirectoryMetadata for the URN. This function
	// can be overriden in order to perform book keeping on
	// itermediate DirectoryMetadata objects.  If it returns
	// errorNoDirectoryMetadata then we skip updating the metadata.
	get_dir_metadata func(
		dir_cache *DirectoryLRUCache,
		db DataStore, config_obj *config_proto.Config,
		urn api.DSPathSpec) (*DirectoryMetadata, error)
}

// Recursively makes sure intermediate directories are created and
// return a DirectoryMetadata object for the urn.
func get_dir_metadata(
	dir_cache *DirectoryLRUCache,
	db DataStore, config_obj *config_proto.Config, urn api.DSPathSpec) (
	*DirectoryMetadata, error) {

	// Check if the top level directory contains metadata.
	path := AsDatastoreDirectory(db, config_obj, urn)
	md, pres := dir_cache.Get(path)
	if pres {
		return md, nil
	}

	// Create top level and every level under it.
	md = NewDirectoryMetadata(dir_cache.max_item_size)
	err := dir_cache.Set(path, md)
	if err != nil {
		return nil, err
	}

	for len(urn.Components()) > 0 {
		parent := urn.Dir()
		path := AsDatastoreDirectory(db, config_obj, parent)

		intermediate_md, ok := dir_cache.Get(path)
		if !ok {
			intermediate_md = NewDirectoryMetadata(dir_cache.max_item_size)
			err := dir_cache.Set(path, intermediate_md)
			if err != nil {
				return nil, err
			}
		}

		key := urn.Base() + api.GetExtensionForDatastore(urn)
		_, pres := intermediate_md.Get(key)
		if !pres {
			// Walk up the directory path.
			intermediate_md.Set(urn)
			urn = parent

		} else {

			// Path is already set we can quit early.
			return md, nil
		}
	}
	return md, nil
}

// Reads a stored message from the datastore. If there is no
// stored message at this URN, the function returns an
// os.ErrNotExist error.
func (self *MemcacheDatastore) GetSubject(
	config_obj *config_proto.Config,
	urn api.DSPathSpec,
	message proto.Message) error {

	defer Instrument("read", "MemcacheDatastore", urn)()

	path := AsDatastoreFilename(self, config_obj, urn)
	bulk_data_any, err := self.data_cache.Get(path)
	if err != nil {
		// Second try the old DB without json. This supports
		// migration from old protobuf based datastore files
		// to newer json based blobs while still being able to
		// read old files.
		if urn.Type() == api.PATH_TYPE_DATASTORE_JSON {
			bulk_data_any, err = self.data_cache.Get(
				AsDatastoreFilename(self, config_obj,
					urn.SetType(api.PATH_TYPE_DATASTORE_PROTO)))
		}

		if err != nil {
			return fmt.Errorf(
				"While opening %v: %w", urn.AsClientPath(),
				utils.NotFoundError)
		}
	}

	// TODO ensure caches are map[string][]byte)
	bulk_data, ok := bulk_data_any.(*BulkData)
	if !ok {
		return internalError
	}

	return unmarshalData(bulk_data.data, urn, message)
}

func unmarshalData(serialized_content []byte,
	urn api.DSPathSpec, message proto.Message) error {
	if len(serialized_content) == 0 {
		return nil
	}

	// It is really a JSON blob
	var err error
	if serialized_content[0] == '{' {
		err = protojson.Unmarshal(serialized_content, message)
	} else {
		err = proto.Unmarshal(serialized_content, message)
	}

	if err != nil {
		return fmt.Errorf("While decoding %v: %w",
			urn.AsClientPath(), utils.NotFoundError)
	}
	return nil
}

func (self *MemcacheDatastore) Healthy() error {
	return nil
}

func (self *MemcacheDatastore) SetTimeout(duration time.Duration) {
	_ = self.data_cache.SetTTL(duration)
	self.data_cache.SkipTTLExtensionOnHit(true)

	_ = self.dir_cache.SetTTL(duration)
	self.dir_cache.SkipTTLExtensionOnHit(true)
}

func (self *MemcacheDatastore) SetCheckExpirationCallback(
	callback ttlcache.CheckExpireCallback) {
	self.data_cache.SetCheckExpirationCallback(callback)
	self.dir_cache.SetCheckExpirationCallback(callback)
}

func (self *MemcacheDatastore) SetSubject(
	config_obj *config_proto.Config,
	urn api.DSPathSpec,
	message proto.Message) error {

	return self.SetSubjectWithCompletion(config_obj, urn, message, nil)
}

func (self *MemcacheDatastore) SetSubjectWithCompletion(
	config_obj *config_proto.Config,
	urn api.DSPathSpec,
	message proto.Message,
	completion func()) error {

	defer Instrument("write", "MemcacheDatastore", urn)()

	// If we were called with utils.SyncCompleter it means we need to
	// wait here until the transaction is flushed to disk.
	if utils.CompareFuncs(completion, utils.SyncCompleter) {
		var wg sync.WaitGroup
		wg.Add(1)
		defer wg.Wait()

		completion = wg.Done
	}

	// Make sure to call the completer on all exit points
	// (MemcacheDatastore is actually synchronous).
	defer func() {
		if completion != nil {
			completion()
		}
	}()

	var value []byte
	var err error

	if urn.Type() == api.PATH_TYPE_DATASTORE_JSON {
		value, err = protojson.Marshal(message)
		if err != nil {
			return err
		}
	} else {
		value, err = proto.Marshal(message)
	}

	if err != nil {
		return err
	}

	return self.SetData(config_obj, urn, value)
}

func (self *MemcacheDatastore) SetData(
	config_obj *config_proto.Config,
	urn api.DSPathSpec, data []byte) (err error) {

	err = self.data_cache.Set(
		AsDatastoreFilename(self, config_obj, urn), &BulkData{
			data: data,
		})
	if err != nil {
		return err
	}

	// Try to update the DirectoryMetadata cache if possible.
	parent := urn.Dir()
	md, err := self.get_dir_metadata(self.dir_cache, self, config_obj, parent)
	if err == errorNoDirectoryMetadata {
		return nil
	}
	if err == nil {

		// There is a valid DirectoryMetadata. Update it with the new
		// child.
		md.Set(urn)
	}
	return err
}

func (self *MemcacheDatastore) DeleteSubjectWithCompletion(
	config_obj *config_proto.Config,
	urn api.DSPathSpec, completion func()) error {

	err := self.DeleteSubject(config_obj, urn)
	if completion != nil &&
		!utils.CompareFuncs(completion, utils.SyncCompleter) {
		completion()
	}

	return err
}

func (self *MemcacheDatastore) DeleteSubject(
	config_obj *config_proto.Config,
	urn api.DSPathSpec) error {
	defer Instrument("delete", "MemcacheDatastore", urn)()

	err := self.data_cache.Remove(
		AsDatastoreFilename(self, config_obj, urn))
	if err != nil {
		return utils.Wrap(utils.NotFoundError, "DeleteSubject")
	}

	// Try to remove it from the DirectoryMetadata if it exists.
	md, err := self.get_dir_metadata(self.dir_cache, self, config_obj, urn.Dir())

	// No DirectoryMetadata, nothing to do.
	if err == errorNoDirectoryMetadata {
		return nil
	}

	if err == nil {
		// Update the directory metadata.
		md.Remove(urn)
		return nil
	}
	return utils.Wrap(utils.NotFoundError, "DeleteSubject")
}

func (self *MemcacheDatastore) SetChildren(
	config_obj *config_proto.Config,
	urn api.DSPathSpec, children []api.DSPathSpec) {

	path := AsDatastoreDirectory(self, config_obj, urn)
	md, pres := self.dir_cache.Get(path)
	if !pres {
		md = self.dir_cache.NewDirectoryMetadata(path)
	}

	// If the directory is full we dont add new children to it.
	if md.IsFull() {
		return
	}

	for _, child := range children {
		md.Set(child)
	}

	_ = self.dir_cache.Set(path, md)
}

// Lists all the children of a URN.
func (self *MemcacheDatastore) ListChildren(
	config_obj *config_proto.Config,
	urn api.DSPathSpec) ([]api.DSPathSpec, error) {

	defer Instrument("list", "MemcacheDatastore", urn)()

	path := AsDatastoreDirectory(self, config_obj, urn)
	md, pres := self.dir_cache.Get(path)
	if !pres {
		return nil, nil
	}

	// Can not list very large directories - but we still cache the
	// fact that they are oversize.
	if md.IsFull() {
		return nil, errorOversize
	}

	result := make([]api.DSPathSpec, 0, md.Len())
	for _, v := range md.Items() {
		result = append(result, v)
	}

	return result, nil
}

// Called to close all db handles etc. Not thread safe.
func (self *MemcacheDatastore) Close() {
	self.data_cache.Flush()
	self.dir_cache.Flush()
}

// Clear the cache and drop the data on the floor.
func (self *MemcacheDatastore) Clear() {
	_ = self.data_cache.Purge()
	_ = self.dir_cache.Purge()
}

func (self *MemcacheDatastore) GetForTests(
	config_obj *config_proto.Config,
	path string) ([]byte, error) {
	components := []string{}
	for _, s := range strings.Split(path, "/") {
		if s != "" {
			components = append(components, s)
		}
	}

	return self.GetBuffer(config_obj,
		path_specs.NewUnsafeDatastorePath(components...))
}

// Support RawDataStore interface
func (self *MemcacheDatastore) GetBuffer(
	config_obj *config_proto.Config,
	urn api.DSPathSpec) ([]byte, error) {
	path := AsDatastoreFilename(self, config_obj, urn)
	bulk_data_any, err := self.data_cache.Get(path)
	bulk_data, ok := bulk_data_any.(*BulkData)
	if !ok {
		return nil, internalError
	}
	bulk_data.mu.Lock()
	defer bulk_data.mu.Unlock()

	return bulk_data.data, err
}

func (self *MemcacheDatastore) SetBuffer(
	config_obj *config_proto.Config,
	urn api.DSPathSpec, data []byte, completion func()) error {

	err := self.SetData(config_obj, urn, data)
	if completion != nil &&
		!utils.CompareFuncs(completion, utils.SyncCompleter) {
		completion()
	}
	return err
}

func (self *MemcacheDatastore) Debug(config_obj *config_proto.Config) {
	for _, key := range self.dir_cache.GetKeys() {
		md, _ := self.dir_cache.Get(key)
		for _, spec := range md.Items() {
			fmt.Printf("%v: %v\n", key, spec.AsClientPath())
		}
	}
}

func (self *MemcacheDatastore) Dump() []api.DSPathSpec {
	result := make([]api.DSPathSpec, 0)

	for _, key := range self.dir_cache.GetKeys() {
		md, _ := self.dir_cache.Get(key)
		for _, spec := range md.Items() {
			result = append(result, spec)
		}
	}
	return result
}

func (self *MemcacheDatastore) SetDirLoader(cb func(
	dir_cache *DirectoryLRUCache,
	db DataStore, config_obj *config_proto.Config,
	urn api.DSPathSpec) (*DirectoryMetadata, error)) {
	self.get_dir_metadata = cb
}

func (self *MemcacheDatastore) Stats() *MemcacheStats {
	return &MemcacheStats{
		DataItemCount: self.data_cache.Count(),
		DataItemSize:  self.data_cache.Size(),
		DirItemCount:  self.dir_cache.Count(),
		DirItemSize:   self.dir_cache.Size(),
	}
}

func NewMemcacheDataStore(
	ctx context.Context, config_obj *config_proto.Config) *MemcacheDatastore {
	// This data store is used for testing so we really do not want to
	// expire anything.
	result := &MemcacheDatastore{
		data_cache:       NewDataLRUCache(ctx, config_obj, 100000, 1000000),
		dir_cache:        NewDirectoryLRUCache(ctx, config_obj, 100000, 100000),
		get_dir_metadata: get_dir_metadata,
	}

	return result
}
