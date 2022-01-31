// +build !windows

package readers

import (
	"errors"
	"sync"

	ntfs "www.velocidex.com/golang/go-ntfs/parser"
	"www.velocidex.com/golang/velociraptor/constants"
	vql_subsystem "www.velocidex.com/golang/velociraptor/vql"
	"www.velocidex.com/golang/vfilter"
)

type NTFSCachedContext struct {
	mu           sync.Mutex
	paged_reader *AccessorReader
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
func GetNTFSContext(scope vfilter.Scope,
	device, accessor string) (*ntfs.NTFSContext, error) {
	key := "ntfsctx_cache" + device + accessor

	// Get the cache context from the root scope's cache
	cache_ctx, ok := vql_subsystem.CacheGet(scope, key).(*ntfs.NTFSContext)
	if ok {
		return cache_ctx, nil
	}

	lru_size := vql_subsystem.GetIntFromRow(scope, scope, constants.NTFS_CACHE_SIZE)
	paged_reader, err := NewPagedReader(scope, accessor, device, int(lru_size))
	if err != nil {
		return nil, err
	}

	// Read the header to make sure we can actually read this file and
	// it is an NTFS file.
	header := make([]byte, 8)
	_, err = paged_reader.ReadAt(header, 3)
	if err != nil {
		return nil, err
	}
	if string(header) != "NTFS    " {
		return nil, errors.New("No NTFS Magic")
	}

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
