package readers

import (
	"context"
	"errors"
	"sync"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"www.velocidex.com/golang/go-ntfs/parser"
	ntfs "www.velocidex.com/golang/go-ntfs/parser"
	"www.velocidex.com/golang/velociraptor/accessors"
	"www.velocidex.com/golang/velociraptor/constants"
	"www.velocidex.com/golang/velociraptor/utils"
	vql_subsystem "www.velocidex.com/golang/velociraptor/vql"
	vql_constants "www.velocidex.com/golang/velociraptor/vql/constants"
	"www.velocidex.com/golang/velociraptor/vql/readers"
	"www.velocidex.com/golang/vfilter"
)

var (
	ntfsCacheTotalOpened = promauto.NewCounter(prometheus.CounterOpts{
		Name: "ntfs_cache_total_open",
		Help: "Total Number of times we opened the ntfs cache",
	})
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

	accessor string
	device   *accessors.OSPath

	device_is_raw_mft bool
	scope             vfilter.Scope
	paged_reader      *readers.AccessorReader
	ntfs_ctx          *ntfs.NTFSContext

	id           uint64
	started      time.Time
	next_refresh time.Time

	// When this is closed we stop refreshing the cache. Normally
	// only closed when the scope is destroyed.
	done chan bool
}

// Close the NTFS context every minute - this forces a refresh and
// reparse of the NTFS device.
func (self *NTFSCachedContext) Start(
	ctx context.Context, scope vfilter.Scope) (err error) {

	cache_life := vql_constants.GetNTFSCacheTime(ctx, scope)

	self.mu.Lock()
	self.started = utils.GetTime().Now()
	self.next_refresh = self.started.Add(cache_life)
	self.mu.Unlock()

	lru_size := vql_subsystem.GetIntFromRow(
		self.scope, self.scope, constants.NTFS_CACHE_SIZE)
	self.paged_reader, err = readers.NewAccessorReader(
		self.scope, self.accessor, self.device, int(lru_size))

	if err != nil {
		return err
	}

	// Read the header to make sure we can actually read the raw
	// device.
	if self.device_is_raw_mft {
		header := make([]byte, 4)
		_, err = self.paged_reader.ReadAt(header, 0)
		if err != nil {
			return err
		}

		if string(header) != "FILE" {
			return errors.New("File does not have an MFT Magic")
		}

	} else {
		header := make([]byte, 8)
		_, err = self.paged_reader.ReadAt(header, 3)
		if err != nil {
			return err
		}

		if string(header) != "NTFS    " {
			return errors.New("No NTFS Magic")
		}
	}

	go func() {
		for {
			select {
			case <-self.done:
				self.mu.Lock()
				self.done = nil
				self.mu.Unlock()
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
	if self.ntfs_ctx != nil {
		self.ntfs_ctx.Close()
		self.ntfs_ctx = nil
		self.started = time.Time{}
		self.next_refresh = self.started
	}
	self.paged_reader.Close()
}

func (self *NTFSCachedContext) detectClusterSize() (int64, int64, error) {
	// We need to detect the cluster size or the MFT entry size. We do
	// this by checking the signature for the MFT entry. Normally this
	// information is given in the boot sector but without the boot
	// sector we make do.
	buf := make([]byte, 4)
	for i := int64(512); i < 8192; i += 512 {
		n, err := self.paged_reader.ReadAt(buf, i)
		if err != nil || n != 4 {
			return 0, 0, err
		}
		if string(buf) == "FILE" {
			return 4096, i, nil
		}
	}

	return 0, 0, errors.New("Unknown MFT Cluster Size")
}

func (self *NTFSCachedContext) GetNTFSContext() (ntfs_ctx *ntfs.NTFSContext, err error) {
	self.mu.Lock()
	defer self.mu.Unlock()

	self.scope.ChargeOp()

	// If the cache is valid just return it.
	if self.ntfs_ctx != nil {
		return self.ntfs_ctx, nil
	}

	if self.device_is_raw_mft {
		cluster_size, record_size, err := self.detectClusterSize()
		if err != nil {
			return nil, err
		}

		ntfs_ctx = ntfs.GetNTFSContextFromRawMFT(
			self.paged_reader, cluster_size, record_size)

	} else {
		ntfs_ctx, err = ntfs.GetNTFSContext(self.paged_reader, 0)
		if err != nil {
			self._CloseWithLock()
			return nil, err
		}
	}

	ntfs_ctx.SetOptions(GetScopeOptions(self.scope))

	self.ntfs_ctx = ntfs_ctx

	return self.ntfs_ctx, nil
}

func GetNTFSContext(scope vfilter.Scope,
	device *accessors.OSPath, accessor string) (*ntfs.NTFSContext, error) {
	result, err := getNTFSCache(scope, device, accessor,
		false /* device_is_raw_mft */)
	if err != nil {
		return nil, err
	}

	return result.GetNTFSContext()
}

func GetNTFSContextFromRawMFT(scope vfilter.Scope,
	mft_filename *accessors.OSPath, accessor string) (*ntfs.NTFSContext, error) {
	result, err := getNTFSCache(scope, mft_filename, accessor,
		true /* device_is_raw_mft */)
	if err != nil {
		return nil, err
	}

	return result.GetNTFSContext()
}

func getNTFSCache(scope vfilter.Scope,
	device *accessors.OSPath, accessor string,
	device_is_raw_mft bool) (*NTFSCachedContext, error) {
	key := "ntfsctx_cache" + device.String() + accessor

	// Get the cache context from the root scope's cache
	cache_ctx, ok := vql_subsystem.CacheGet(scope, key).(*NTFSCachedContext)
	if !ok {
		// Create a new cache context.

		cache_ctx = &NTFSCachedContext{
			accessor:          accessor,
			device:            device,
			device_is_raw_mft: device_is_raw_mft,
			scope:             scope,
			done:              make(chan bool),
			id:                utils.GetId(),
		}

		err := cache_ctx.Start(context.Background(), scope)
		if err != nil {
			return nil, err
		}

		Tracker.Register(cache_ctx)

		// Destroy the context when the scope is done.
		err = vql_subsystem.GetRootScope(scope).AddDestructor(func() {
			cache_ctx.mu.Lock()
			if cache_ctx.done != nil {
				close(cache_ctx.done)
			}
			cache_ctx.mu.Unlock()
			cache_ctx.Close()
			Tracker.Unregister(cache_ctx)
		})
		if err != nil {
			return nil, err
		}
		vql_subsystem.CacheSet(scope, key, cache_ctx)
		ntfsCacheTotalOpened.Inc()
	}

	return cache_ctx, nil
}

func GetScopeOptions(scope vfilter.Scope) parser.Options {
	directory_depth := vql_subsystem.GetIntFromRow(
		scope, scope, constants.NTFS_MAX_DIRECTORY_DEPTH)
	if directory_depth == 0 {
		directory_depth = 20
	}

	max_links := vql_subsystem.GetIntFromRow(
		scope, scope, constants.NTFS_MAX_LINKS)
	if max_links == 0 {
		max_links = 20
	}

	include_short_names := vql_subsystem.GetBoolFromRow(
		scope, scope, constants.NTFS_INCLUDE_SHORT_NAMES)

	full_path_resolution := vql_subsystem.GetBoolFromRow(
		scope, scope, constants.NTFS_DISABLE_FULL_PATH_RESOLUTION)

	return parser.Options{
		MaxDirectoryDepth:         int(directory_depth),
		MaxLinks:                  int(max_links),
		IncludeShortNames:         include_short_names,
		DisableFullPathResolution: full_path_resolution,
	}
}
