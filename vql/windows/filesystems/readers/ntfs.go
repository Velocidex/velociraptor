// +build !windows

package readers

import (
	"sync"

	ntfs "www.velocidex.com/golang/go-ntfs/parser"
	vql_subsystem "www.velocidex.com/golang/velociraptor/vql"
	"www.velocidex.com/golang/velociraptor/vql/readers"
	"www.velocidex.com/golang/vfilter"
)

type NTFSCachedContext struct {
	mu           sync.Mutex
	paged_reader *readers.AccessorReader
	ntfs_ctx     *ntfs.NTFSContext
}

func (self *NTFSCachedContext) Close() {
	self.mu.Lock()
	defer self.mu.Unlock()

	if self.paged_reader != nil {
		self.paged_reader.Close()
		self.paged_reader = nil
		self.ntfs_ctx = nil
	}
}

// For non -windows we just create a regular caching context
func GetNTFSContext(scope vfilter.Scope, device string) (*ntfs.NTFSContext, error) {
	key := "ntfsctx_cache" + device

	// Get the cache context from the root scope's cache
	cache_ctx, ok := vql_subsystem.CacheGet(scope, key).(*ntfs.NTFSContext)
	if ok {
		return cache_ctx, nil
	}

	paged_reader := readers.NewPagedReader(scope, "file", device)
	ntfs_ctx, err := ntfs.GetNTFSContext(paged_reader, 0)
	if err != nil {
		paged_reader.Close()
		return nil, err
	}

	// Destroy the context when the scope is done.
	err = vql_subsystem.GetRootScope(scope).AddDestructor(paged_reader.Close)
	if err != nil {
		return nil, err
	}
	vql_subsystem.CacheSet(scope, key, ntfs_ctx)
	return ntfs_ctx, nil
}
