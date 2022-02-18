package ntfs

import (
	"strings"
	"time"

	"www.velocidex.com/golang/velociraptor/accessors"
	"www.velocidex.com/golang/velociraptor/constants"
	"www.velocidex.com/golang/velociraptor/third_party/cache"
	vql_subsystem "www.velocidex.com/golang/velociraptor/vql"
	"www.velocidex.com/golang/vfilter"
)

// An LRU cache of ntfs path and their directry listing. This is
// needed in order to quickly resolve full paths for mft entries by
// following the parent mft reference.

type CacheMFT struct {
	Component string
	MftId     int64
	NameType  string
}

type cacheElement struct {
	children map[string]*CacheMFT
}

func (self cacheElement) Size() int {
	return 1
}

type NTFSPathCache struct {
	scope        vfilter.Scope
	path_listing *cache.LRUCache
	done         chan bool
}

// Close the NTFS context every minute - this forces a refresh and
// reparse of the NTFS device.
func (self *NTFSPathCache) Start(scope vfilter.Scope) {
	cache_life := vql_subsystem.GetIntFromRow(
		scope, scope, constants.NTFS_CACHE_TIME)
	if cache_life == 0 {
		cache_life = 60
	}

	go func() {
		for {
			select {
			case <-self.done:
				return

			case <-time.After(time.Duration(cache_life) * time.Second):
				self.path_listing.Clear()
			}
		}
	}()
}

func (self *NTFSPathCache) SetLRUMap(path string, lru_map map[string]*CacheMFT) {
	self.path_listing.Set(path, cacheElement{children: lru_map})
}

// Query the cache for the MFT metadata of a directory path and a
// component within it.
func (self *NTFSPathCache) GetComponentMetadata(dirpath string, component string) (*CacheMFT, bool) {
	value, pres := self.path_listing.Get(dirpath)
	if pres {
		item, pres := value.(cacheElement).children[strings.ToLower(component)]
		return item, pres
	}

	return nil, false
}

func (self *NTFSPathCache) GetDirLRU(dirpath string) (map[string]*CacheMFT, bool) {
	res, pres := self.path_listing.Get(dirpath)
	if !pres {
		return nil, false
	}

	return res.(cacheElement).children, true
}

func GetNTFSPathCache(scope vfilter.Scope,
	device *accessors.OSPath, accessor string) *NTFSPathCache {
	key := "ntfs_path_cache" + device.String() + accessor

	// Get the cache context from the root scope's cache
	cache_ctx, ok := vql_subsystem.CacheGet(scope, key).(*NTFSPathCache)
	if !ok {
		cache_ctx = &NTFSPathCache{
			path_listing: cache.NewLRUCache(200),
			scope:        scope,
		}
		vql_subsystem.CacheSet(scope, key, cache_ctx)
	}
	return cache_ctx
}
