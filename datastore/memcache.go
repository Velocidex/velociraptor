package datastore

import (
	"fmt"
	"os"
	"sort"
	"sync"
	"time"

	"github.com/pkg/errors"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"

	"github.com/ReneKroon/ttlcache/v2"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	"www.velocidex.com/golang/velociraptor/file_store/api"
)

var (
	memcache_imp = NewMemcacheDataStore()

	internalError = errors.New("Internal datastore error")
	errorNotFound = errors.New("Not found")

	metricDirLRU = promauto.NewGauge(
		prometheus.GaugeOpts{
			Name: "memcache_dir_lru_total",
			Help: "Total directories cached",
		})

	metricDataLRU = promauto.NewGauge(
		prometheus.GaugeOpts{
			Name: "memcache_data_lru_total",
			Help: "Total files cached",
		})
)

// Stored in data_cache contains bulk data.
type BulkData struct {
	mu   sync.Mutex
	data []byte
}

// Stored in dir_cache - contains DirectoryMetadata
type DirectoryMetadata struct {
	mu   sync.Mutex
	data map[string]api.DSPathSpec
}

func (self *DirectoryMetadata) Set(key string, value api.DSPathSpec) {
	self.mu.Lock()
	defer self.mu.Unlock()

	self.data[key] = value
}

func (self *DirectoryMetadata) Remove(key string) {
	self.mu.Lock()
	defer self.mu.Unlock()

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

func NewDirectoryMetadata() *DirectoryMetadata {
	return &DirectoryMetadata{
		data: make(map[string]api.DSPathSpec),
	}
}

type DirectoryLRUCache struct {
	*ttlcache.Cache
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

func NewDirectoryLRUCache() *DirectoryLRUCache {
	result := &DirectoryLRUCache{
		Cache: ttlcache.NewCache(),
	}

	result.Cache.SetNewItemCallback(func(key string, value interface{}) {
		metricDirLRU.Inc()
	})

	result.Cache.SetExpirationCallback(func(key string, value interface{}) {
		metricDirLRU.Dec()
	})

	return result
}

// This is a memory cached data store.
type MemcacheDatastore struct {
	// Stores data like key value
	data_cache *ttlcache.Cache

	// Stores directory metadata.
	dir_cache *DirectoryLRUCache

	// A function to update directory caches
	get_dir_metadata func(
		dir_cache *DirectoryLRUCache,
		config_obj *config_proto.Config,
		urn api.DSPathSpec) (*DirectoryMetadata, error)
}

// Recursively makes sure the directories are created.
func get_dir_metadata(
	dir_cache *DirectoryLRUCache,
	config_obj *config_proto.Config, urn api.DSPathSpec) (
	*DirectoryMetadata, error) {

	// Check if the top level directory contains metadata.
	path := urn.AsDatastoreDirectory(config_obj)
	md, pres := dir_cache.Get(path)
	if pres {
		return md, nil
	}

	// Create top level and every level under it.
	md = NewDirectoryMetadata()
	dir_cache.Set(path, md)

	for len(urn.Components()) > 0 {
		parent := urn.Dir()
		path := parent.AsDatastoreDirectory(config_obj)

		intermediate_md, ok := dir_cache.Get(path)
		if !ok {
			intermediate_md = NewDirectoryMetadata()
			dir_cache.Set(path, intermediate_md)
		}

		key := urn.Base() + api.GetExtensionForDatastore(urn)
		_, pres := intermediate_md.Get(key)
		if !pres {
			// Walk up the directory path.
			intermediate_md.Set(key, urn)
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

	defer Instrument("read", urn)()

	path := urn.AsClientPath()
	bulk_data_any, err := self.data_cache.Get(path)
	if err != nil {
		// Second try the old DB without json. This supports
		// migration from old protobuf based datastore files
		// to newer json based blobs while still being able to
		// read old files.
		if urn.Type() == api.PATH_TYPE_DATASTORE_JSON {
			bulk_data_any, err = self.data_cache.Get(
				urn.SetType(api.PATH_TYPE_DATASTORE_PROTO).AsClientPath())
		}

		if err != nil {
			return errors.WithMessage(os.ErrNotExist,
				fmt.Sprintf("While opening %v: not found", urn.AsClientPath()))
		}
	}

	// TODO ensure caches are map[string][]byte)
	bulk_data, ok := bulk_data_any.(*BulkData)
	if !ok {
		return internalError
	}

	serialized_content := bulk_data.data
	if len(serialized_content) == 0 {
		return nil
	}

	// It is really a JSON blob
	if serialized_content[0] == '{' {
		err = protojson.Unmarshal(serialized_content, message)
	} else {
		err = proto.Unmarshal(serialized_content, message)
	}

	if err != nil {
		return errors.WithMessage(os.ErrNotExist,
			fmt.Sprintf("While opening %v: %v",
				urn.AsClientPath(), err))
	}
	return nil
}

func (self *MemcacheDatastore) SetTimeout(duration time.Duration) {
	self.data_cache.SetTTL(duration)
	self.dir_cache.SetTTL(duration)
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

	defer Instrument("write", urn)()

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
	urn api.DSPathSpec,
	data []byte) (err error) {

	// Get new dir metadata
	md, err := self.get_dir_metadata(self.dir_cache, config_obj, urn.Dir())
	if err != nil {
		return err
	}

	// Update the directory metadata.
	md_key := urn.Base() + api.GetExtensionForDatastore(urn)
	md.Set(md_key, urn)

	// Update the cache
	parent_path := urn.Dir().AsClientPath()
	self.dir_cache.Set(parent_path, md)
	return self.data_cache.Set(urn.AsClientPath(), &BulkData{
		data: data,
	})
}

func (self *MemcacheDatastore) DeleteSubject(
	config_obj *config_proto.Config,
	urn api.DSPathSpec) error {
	defer Instrument("delete", urn)()

	err := self.data_cache.Remove(urn.AsClientPath())
	if err != nil {
		return err
	}

	// Get new dir metadata
	md, err := self.get_dir_metadata(self.dir_cache, config_obj, urn.Dir())
	if err != nil {
		return err
	}

	// Update the directory metadata.
	md_key := urn.Base() + api.GetExtensionForDatastore(urn)
	md.Remove(md_key)

	// Update the cache
	parent_path := urn.Dir().AsClientPath()
	self.dir_cache.Set(parent_path, md)

	return nil
}

func (self *MemcacheDatastore) SetChildren(
	config_obj *config_proto.Config,
	urn api.DSPathSpec, children []api.DSPathSpec) {

	path := urn.AsDatastoreDirectory(config_obj)

	md := &DirectoryMetadata{
		data: make(map[string]api.DSPathSpec),
	}

	for _, child := range children {
		key := child.Base() + api.GetExtensionForDatastore(child)
		md.Set(key, child)
	}

	self.dir_cache.Set(path, md)
}

// Lists all the children of a URN.
func (self *MemcacheDatastore) ListChildren(
	config_obj *config_proto.Config,
	urn api.DSPathSpec) ([]api.DSPathSpec, error) {

	defer Instrument("list", urn)()

	path := urn.AsDatastoreDirectory(config_obj)
	md, pres := self.dir_cache.Get(path)
	if !pres {
		return nil, nil
	}

	result := make([]api.DSPathSpec, 0, md.Len())
	for _, v := range md.Items() {
		result = append(result, v)
	}

	return result, nil
}

func (self *MemcacheDatastore) Walk(config_obj *config_proto.Config,
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

// Called to close all db handles etc. Not thread safe.
func (self *MemcacheDatastore) Close() {}

func (self *MemcacheDatastore) Clear() {
	self.data_cache.Purge()
	self.dir_cache.Purge()
}

// Support RawDataStore interface
func (self *MemcacheDatastore) GetBuffer(
	config_obj *config_proto.Config,
	urn api.DSPathSpec) ([]byte, error) {
	path := urn.AsClientPath()
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
	urn api.DSPathSpec, data []byte) error {

	return self.SetData(config_obj, urn, data)
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
	config_obj *config_proto.Config,
	urn api.DSPathSpec) (*DirectoryMetadata, error)) {
	self.get_dir_metadata = cb
}

func NewMemcacheDataStore() *MemcacheDatastore {
	result := &MemcacheDatastore{
		data_cache:       ttlcache.NewCache(),
		dir_cache:        NewDirectoryLRUCache(),
		get_dir_metadata: get_dir_metadata,
	}

	result.data_cache.SetNewItemCallback(func(key string, value interface{}) {
		metricDataLRU.Inc()
	})

	result.data_cache.SetExpirationCallback(func(key string, value interface{}) {
		metricDataLRU.Dec()
	})

	return result
}
