package readers

import (
	"context"
	"errors"
	"sync"
	"time"

	ntfs "www.velocidex.com/golang/go-ntfs/parser"
	"www.velocidex.com/golang/velociraptor/constants"
	vql_subsystem "www.velocidex.com/golang/velociraptor/vql"
	vql_constants "www.velocidex.com/golang/velociraptor/vql/constants"
	"www.velocidex.com/golang/velociraptor/vql/readers"
	"www.velocidex.com/golang/vfilter"
)

// The NTFS parser is responsible for extracting artifacts from
// NTFS. We need to balance two competing needs:

// 1. We should not read too often and prefer to cache frequently
//    accessed sectors in memory as they will be traversed over and
//    over (e.g. the $MFT is always looked up).

// 2. For very long running queries we do not want to cache too long
//    or we will be unable to get new data (think event queries).

// Further we want to destroy the ntfs cache when a query terminates
// so we can free up memory. In practice it is hard to match the
// lifetime of the cache with the lifetime of the scope - the query
// could be quick or take a long time. Closing the underlying file at
// the wrong time can cause issues with the query trying to access it.

// This reader manages the NTFS Context lifetime in the root scope's
// cache. The idea being that it should be safe to close the
// underlying file at any time. If anyone attempts to access the file,
// the file can be reopened and the ntfs context reparsed on demand.

// Manage the cache of the NTFS parser - may be shared by multiple
// threads. Contains all the information required to re-open the
// underlying file.
type NTFSCachedContext struct {
	mu sync.Mutex

	accessor     string
	device       string
	scope        vfilter.Scope
	paged_reader *readers.AccessorReader
	ntfs_ctx     *ntfs.NTFSContext
	lru_size     int

	// When this is closed we stop refreshing the cache. Normally
	// only closed when the scope is destroyed.
	done chan bool
}

// Close the NTFS context every minute - this forces a refresh and
// reparse of the NTFS device.
func (self *NTFSCachedContext) Start(
	ctx context.Context, scope vfilter.Scope) (err error) {

	cache_life := vql_constants.GetNTFSCacheTime(ctx, scope)
	done := self.done

	lru_size := vql_subsystem.GetIntFromRow(
		self.scope, self.scope, constants.NTFS_CACHE_SIZE)
	self.paged_reader, err = readers.NewPagedReader(
		self.scope, self.accessor, self.device, int(lru_size))

	if err != nil {
		return err
	}

	// Read the header to make sure we can actually read the raw
	// device.
	header := make([]byte, 8)
	_, err = self.paged_reader.ReadAt(header, 3)
	if err != nil {
		return err
	}

	if string(header) != "NTFS    " {
		return errors.New("No NTFS Magic")
	}

	go func() {
		for {
			select {
			case <-done:
				return

			case <-time.After(cache_life):
				self.Close()
			}
		}
	}()

	return err
}

// Close may be called multiple times and at any time.
func (self *NTFSCachedContext) Close() {
	self.mu.Lock()
	defer self.mu.Unlock()

	self._CloseWithLock()
}

func (self *NTFSCachedContext) _CloseWithLock() {
	self.paged_reader.Close()
}

func (self *NTFSCachedContext) GetNTFSContext() (*ntfs.NTFSContext, error) {
	self.mu.Lock()
	defer self.mu.Unlock()

	// If the cache is valid just return it.
	if self.ntfs_ctx != nil {
		return self.ntfs_ctx, nil
	}

	ntfs_ctx, err := ntfs.GetNTFSContext(self.paged_reader, 0)
	if err != nil {
		self._CloseWithLock()
		return nil, err
	}

	self.ntfs_ctx = ntfs_ctx

	return self.ntfs_ctx, nil
}

func GetNTFSContext(scope vfilter.Scope, device, accessor string) (*ntfs.NTFSContext, error) {
	result, err := GetNTFSCache(scope, device, accessor)
	if err != nil {
		return nil, err
	}

	return result.GetNTFSContext()
}

func GetNTFSCache(scope vfilter.Scope, device, accessor string) (*NTFSCachedContext, error) {
	key := "ntfsctx_cache" + device + accessor

	// Get the cache context from the root scope's cache
	cache_ctx, ok := vql_subsystem.CacheGet(scope, key).(*NTFSCachedContext)
	if !ok {
		cache_ctx = &NTFSCachedContext{
			accessor: accessor,
			device:   device,
			scope:    scope,
		}
		err := cache_ctx.Start(context.Background(), scope)
		if err != nil {
			return nil, err
		}

		// Destroy the context when the scope is done.
		err = vql_subsystem.GetRootScope(scope).AddDestructor(func() {
			cache_ctx.mu.Lock()
			if cache_ctx.done != nil {
				close(cache_ctx.done)
			}
			cache_ctx.mu.Unlock()
			cache_ctx.Close()
		})
		if err != nil {
			return nil, err
		}
		vql_subsystem.CacheSet(scope, key, cache_ctx)
	}

	return cache_ctx, nil
}
