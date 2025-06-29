package vmdk

import (
	"io"
	"sync"

	"github.com/Velocidex/go-vmdk/parser"
	"www.velocidex.com/golang/velociraptor/accessors"
	"www.velocidex.com/golang/velociraptor/json"
	"www.velocidex.com/golang/velociraptor/utils"
	vql_subsystem "www.velocidex.com/golang/velociraptor/vql"
	"www.velocidex.com/golang/vfilter"
)

const (
	VMDK_CACHE_TAG = "__VMDK_CACHE"
)

// Don't bother expiring this until the end of the query.
type vmdkCache struct {
	mu sync.Mutex

	cache map[string]*VMDKFile
}

func (self *vmdkCache) Get(key string) (*VMDKFile, bool) {
	self.mu.Lock()
	defer self.mu.Unlock()

	r, pres := self.cache[key]
	return r, pres
}

func (self *vmdkCache) Set(key string, r *VMDKFile) {
	self.mu.Lock()
	defer self.mu.Unlock()

	self.cache[key] = r
}

func (self *vmdkCache) Close() {
	self.mu.Lock()
	defer self.mu.Unlock()

	for _, r := range self.cache {
		if r.closer != nil {
			r.closer()
		}
	}
}

func getCachedVMDKFile(
	full_path *accessors.OSPath,
	accessor accessors.FileSystemAccessor,
	scope vfilter.Scope) (*VMDKFile, error) {

	cache, pres := vql_subsystem.CacheGet(scope, VMDK_CACHE_TAG).(*vmdkCache)
	if !pres {
		cache = &vmdkCache{
			cache: make(map[string]*VMDKFile),
		}
		// Cache will remain alive for the duration of the query.
		err := vql_subsystem.GetRootScope(scope).AddDestructor(cache.Close)
		if err != nil {
			cache.Close()
			return nil, err
		}

		vql_subsystem.CacheSet(scope, VMDK_CACHE_TAG, cache)
	}

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

	vmdk_ctx, err := parser.GetVMDKContext(
		utils.MakeReaderAtter(fd), 40960,
		func(filename string) (reader io.ReaderAt, closer func(), err error) {
			full_path := delegate.Dirname().Append(filename)
			fd, err := accessor.OpenWithOSPath(full_path)
			if err != nil {
				return nil, nil, err
			}
			return utils.MakeReaderAtter(fd),
				func() { fd.Close() }, nil
		})
	if err != nil {
		return nil, err
	}

	vmdk_file := &VMDKFile{
		reader: vmdk_ctx,
		size:   uint64(vmdk_ctx.Size()),
		closer: func() {
			scope.Log("vmdk: Closing VMDK file %v\n", key)
			fd.Close()
		},
	}

	cache.Set(key, vmdk_file)

	stats := vmdk_ctx.Stats()
	scope.Log("DEBUG:vmdk: Opened VMDK file %v with %v extents: %v\n",
		key, len(stats.Extents), json.MustMarshalString(stats))

	return vmdk_file, nil
}
