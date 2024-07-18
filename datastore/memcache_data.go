package datastore

import (
	"context"
	"sync"

	"github.com/Velocidex/ttlcache/v2"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
)

// Cache data content in memory.
type DataLRUCache struct {
	mu sync.Mutex

	*ttlcache.Cache

	// Max size of cached items
	max_item_size int
}

// Size total cached items.
func (self *DataLRUCache) Size() int {
	self.mu.Lock()
	defer self.mu.Unlock()

	size := 0
	for _, key := range self.GetKeys() {
		bd_any, err := self.Get(key)
		if err == nil {
			bd, ok := bd_any.(*BulkData)
			if ok {
				size += bd.Len()
			}
		}
	}
	return size
}

// Count of cached items.
func (self *DataLRUCache) Count() int {
	self.mu.Lock()
	defer self.mu.Unlock()

	return len(self.GetKeys())
}

// Sets a new item in the cache.
func (self *DataLRUCache) Set(key string, value *BulkData) error {
	self.mu.Lock()
	defer self.mu.Unlock()

	// Skip caching very large bulk data.
	if value.Len() > self.max_item_size {
		return nil
	}

	return self.Cache.Set(key, value)
}

func NewDataLRUCache(
	ctx context.Context, config_obj *config_proto.Config,
	data_max_size, data_max_item_size int) *DataLRUCache {

	result := &DataLRUCache{
		Cache:         ttlcache.NewCache(),
		max_item_size: data_max_item_size,
	}

	result.SetCacheSizeLimit(data_max_size)

	go func() {
		<-ctx.Done()
		result.Cache.Close()
	}()

	return result
}
