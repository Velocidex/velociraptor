package vhdx

import (
	"sync"

	"github.com/Velocidex/go-vhdx/parser"
	"www.velocidex.com/golang/velociraptor/accessors"
	"www.velocidex.com/golang/velociraptor/utils"
	vql_subsystem "www.velocidex.com/golang/velociraptor/vql"
	"www.velocidex.com/golang/vfilter"
)

const (
	VHDX_CACHE_TAG = "__VHDX_CACHE"
)

// Don't bother expiring this until the end of the query.
type vhdxCache struct {
	mu sync.Mutex

	cache map[string]*VHDXFile
}

func (self *vhdxCache) Get(key string) (*VHDXFile, bool) {
	self.mu.Lock()
	defer self.mu.Unlock()

	r, pres := self.cache[key]
	return r, pres
}

func (self *vhdxCache) Set(key string, r *VHDXFile) {
	self.mu.Lock()
	defer self.mu.Unlock()

	self.cache[key] = r
}

func (self *vhdxCache) Close() {
	self.mu.Lock()
	defer self.mu.Unlock()

	for _, r := range self.cache {
		if r.closer != nil {
			r.closer()
		}
	}
}

func getCachedVHDXFile(
	full_path *accessors.OSPath,
	accessor accessors.FileSystemAccessor,
	scope vfilter.Scope) (*VHDXFile, error) {

	cache, pres := vql_subsystem.CacheGet(scope, VHDX_CACHE_TAG).(*vhdxCache)
	if !pres {
		cache = &vhdxCache{
			cache: make(map[string]*VHDXFile),
		}
		// Cache will remain alive for the duration of the query.
		err := vql_subsystem.GetRootScope(scope).AddDestructor(cache.Close)
		if err != nil {
			cache.Close()
			return nil, err
		}
		vql_subsystem.CacheSet(scope, VHDX_CACHE_TAG, cache)
	}

	now := utils.GetTime().Now()
	key := full_path.String()
	res, pres := cache.Get(key)
	if pres {
		// Give a copy of the cache object so it can be seeked
		// independently.
		return res._Copy(), nil
	}

	delegate, err := full_path.Delegate(scope)
	if err != nil {
		return nil, err
	}

	fd, err := accessor.OpenWithOSPath(delegate)
	if err != nil {
		return nil, err
	}

	file_obj, err := parser.NewVHDXFile(utils.MakeReaderAtter(fd))
	if err != nil {
		return nil, err
	}

	vhdx_file := &VHDXFile{
		reader: file_obj,
		size:   file_obj.Metadata.VirtualDiskSize,
		closer: func() {
			scope.Log("vhdx: Closing VHDX file %v\n", key)
			fd.Close()
		},
	}

	cache.Set(key, vhdx_file)
	scope.Log("vhdx: Opened VHDX file %v in %v\n", key,
		utils.GetTime().Now().Sub(now).String())

	return vhdx_file, nil
}
