package datastore

import (
	"fmt"
	"os"
	"sort"
	"sync"

	"github.com/pkg/errors"

	"github.com/ReneKroon/ttlcache/v2"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	"www.velocidex.com/golang/velociraptor/file_store/api"
	"www.velocidex.com/golang/velociraptor/utils"
)

var (
	memcache_imp = &MemcacheDatastore{
		clock:      utils.RealClock{},
		data_cache: ttlcache.NewCache(),
		dir_cache:  ttlcache.NewCache(),
	}

	internalError = errors.New("Internal datastore error")
)

type DirectoryMetadata struct {
	mu   sync.Mutex
	data map[string]api.DSPathSpec
}

func (self *DirectoryMetadata) Set(key string, value api.DSPathSpec) {
	self.mu.Lock()
	defer self.mu.Unlock()

	self.data[key] = value
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

// This is a memory cached data store.
type MemcacheDatastore struct {
	clock utils.Clock

	// Stores data like key value
	data_cache *ttlcache.Cache

	// Stores directory metadata.
	dir_cache *ttlcache.Cache
}

// Recursively makes sure the directories are created
func (self *MemcacheDatastore) mkdirall(
	config_obj *config_proto.Config, urn api.DSPathSpec) {

	// Check if the top level directory contains metadata.
	path := urn.AsDatastoreDirectory(config_obj)
	_, err := self.dir_cache.Get(path)
	if err == nil {
		return
	}

	// Create top level and every level under it.
	self.dir_cache.Set(path, NewDirectoryMetadata())
	for len(urn.Components()) > 0 {
		parent := urn.Dir()
		path := parent.AsDatastoreDirectory(config_obj)

		md_any, err := self.dir_cache.Get(path)
		if err != nil {
			md_any = NewDirectoryMetadata()
			self.dir_cache.Set(path, md_any)
		}

		md := md_any.(*DirectoryMetadata)

		_, pres := md.Get(urn.Base())
		if !pres {
			// Walk up the directory path.
			md.Set(urn.Base(), urn)
			urn = parent

		} else {
			// Path is already set we can quit early.
			return
		}
	}
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
	serialized_content_any, err := self.data_cache.Get(path)
	if err != nil {
		// Second try the old DB without json. This supports
		// migration from old protobuf based datastore files
		// to newer json based blobs while still being able to
		// read old files.
		if urn.Type() == api.PATH_TYPE_DATASTORE_JSON {
			serialized_content_any, err = self.data_cache.Get(
				urn.SetType(api.PATH_TYPE_DATASTORE_PROTO).AsClientPath())
		}

		if err != nil {
			return errors.WithMessage(os.ErrNotExist,
				fmt.Sprintf("While opening %v: not found", urn.AsClientPath()))
		}
	}

	// TODO ensure caches are map[string][]byte)
	serialized_content, ok := serialized_content_any.([]byte)
	if !ok {
		return internalError
	}

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

	parent := urn.Dir()
	parent_path := parent.AsDatastoreDirectory(config_obj)
	md_any, err := self.dir_cache.Get(parent_path)
	if err != nil {
		// Make all intermediate directories.
		self.mkdirall(config_obj, parent)

		// This time this should work.
		md_any, err = self.dir_cache.Get(parent_path)
		if err != nil {
			return err
		}
	}

	// Update the directory metadata.
	md := md_any.(*DirectoryMetadata)
	md_key := urn.Base() + api.GetExtensionForDatastore(urn)
	md.Set(md_key, urn)

	return self.data_cache.Set(urn.AsClientPath(), value)
}

func (self *MemcacheDatastore) DeleteSubject(
	config_obj *config_proto.Config,
	urn api.DSPathSpec) error {
	defer Instrument("delete", urn)()

	return self.data_cache.Remove(urn.AsClientPath())
}

// Lists all the children of a URN.
func (self *MemcacheDatastore) ListChildren(
	config_obj *config_proto.Config,
	urn api.DSPathSpec) ([]api.DSPathSpec, error) {

	defer Instrument("list", urn)()

	path := urn.AsDatastoreDirectory(config_obj)
	md_any, err := self.dir_cache.Get(path)
	if err != nil {
		self.mkdirall(config_obj, urn)
		md_any, err = self.dir_cache.Get(path)
		if err != nil {
			return nil, err
		}
	}

	md := md_any.(*DirectoryMetadata)

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

func (self *MemcacheDatastore) Debug(config_obj *config_proto.Config) {
	for _, key := range self.dir_cache.GetKeys() {
		md_any, _ := self.dir_cache.Get(key)
		md := md_any.(*DirectoryMetadata)
		for _, spec := range md.Items() {
			fmt.Printf("%v: %v\n", key, spec.AsClientPath())
		}
	}
}

func (self *MemcacheDatastore) Dump() []api.DSPathSpec {
	result := make([]api.DSPathSpec, 0)

	for _, key := range self.dir_cache.GetKeys() {
		md_any, _ := self.dir_cache.Get(key)
		md := md_any.(*DirectoryMetadata)
		for _, spec := range md.Items() {
			result = append(result, spec)
		}
	}
	return result
}

func NewMemcacheDataStore(config_obj *config_proto.Config) *MemcacheDatastore {
	return &MemcacheDatastore{
		clock:      utils.RealClock{},
		data_cache: ttlcache.NewCache(),
		dir_cache:  ttlcache.NewCache(),
	}
}
