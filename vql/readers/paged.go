/*
A general purpose cached reader pool

Can be used by any plugins that wish to return references to an open
accessor/file set. We maintain an LRU of paged readers so when
another plugin wants to read the same file, we can immediately
serve it with a cached paged reader.

Note that if the reader is evicted from the LRU, this is not an
error - the reader will simply be recreated on demand by re-opening
the file. This controls the number of concurrent open files so it
is not too large, but still maintains a good temporally correlated
cache.

Depending on the query it is difficult to know when to close the
files based solely on scope. Consider a parser which returns a lazy
object:

SELECT parse_binary(...) FROM glob(globs=..)

The parse_binary() function will return an object wrapping the file
- i.e. it will have a reference to the reader. Depending on the
query, the reader might be accessed at any time. It is difficult to
know when is it safe to remove the file reference - at the end of
the row? at the end the root scope?

Having an LRU allows us to be flexible and not worry about the
scope lifetime so much. Files will eventually get closed and caches
will be evicted.
*/
package readers

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/Velocidex/ordereddict"
	"github.com/Velocidex/ttlcache/v2"
	ntfs "www.velocidex.com/golang/go-ntfs/parser"
	"www.velocidex.com/golang/velociraptor/accessors"
	"www.velocidex.com/golang/velociraptor/utils"
	vql_subsystem "www.velocidex.com/golang/velociraptor/vql"
	"www.velocidex.com/golang/vfilter"
)

// A reader pool is stored in the scope and contains an LRU of
// readers. Each reader wraps an existing reader which may be closed
// at any time. The underlying reader has limited lifetime and will
// close itself after a while as well. This design means that no file
// handles are leaked:

// 1. When the scope is destroyed all readers are closed immediately
// 2. For long running queries, the paged readers themselves will
//    close the underlying file handles.
// 3. Any code that holds a reference to an AccessorReader may use it
//    at any time - even after it was closed. This simplifies lifetime
//    management considerably

// Scope Context variable for the ReaderPool
const READERS_CACHE = "$accessor_reader"

type ReaderPool struct {
	lru *ttlcache.Cache
}

// Moves the reader to the head of the LRU.
func (self *ReaderPool) Activate(reader *AccessorReader) {
	_ = self.lru.Set(reader.Key(), reader)
}

// Flush all contained readers.
func (self *ReaderPool) Close() {
	for _, k := range self.lru.GetKeys() {
		_ = self.lru.Remove(k)
	}
	self.lru.Close()
}

type AccessorReader struct {
	mu sync.Mutex

	Accessor string
	File     *accessors.OSPath
	Scope    vfilter.Scope

	key      string
	max_size int64

	reader       accessors.ReadSeekCloser
	paged_reader *ntfs.PagedReader

	// Owner pool
	pool *ReaderPool

	// Called to cancel the timed closer.
	cancel func()

	// How long to keep the file handle open
	Lifetime time.Duration
	lru_size int

	last_opened time.Time
}

func (self *AccessorReader) Stats() *ordereddict.Dict {
	self.mu.Lock()
	defer self.mu.Unlock()

	var paged_stats *ordereddict.Dict
	if self.paged_reader != nil {
		paged_stats = self.paged_reader.Stats()
	}

	last_opened := ""
	if !self.last_opened.IsZero() {
		last_opened = utils.GetTime().Now().Sub(self.last_opened).
			Round(time.Second).String()
	}

	return ordereddict.NewDict().
		Set("MaxSize", self.max_size).

		// If the underlying reader is closed we are not active but
		// are ready to reopen it on demand.
		Set("Active", self.reader != nil).

		// The reader will force close the underlying reader after
		// this much time.
		Set("Lifetime", self.Lifetime.Round(time.Second).String()).
		Set("LastOpened", last_opened).
		Set("PageCache", paged_stats)

}

func (self *AccessorReader) DebugString() string {
	return fmt.Sprintf("AccessorReader %v: %v\n",
		self.Accessor, self.File.String())
}

func (self *AccessorReader) SetLifetime(l time.Duration) {
	self.mu.Lock()
	defer self.mu.Unlock()

	self.Lifetime = l
}

func (self *AccessorReader) GetLifetime() time.Duration {
	self.mu.Lock()
	defer self.mu.Unlock()

	return self.Lifetime
}

func (self *AccessorReader) Size() int {
	return 1
}

func (self *AccessorReader) MaxSize() int64 {
	return self.max_size
}

func (self *AccessorReader) Key() string {
	return self.key
}

func (self *AccessorReader) Flush() {
	self.Close()
}

func (self *AccessorReader) Close() error {
	self.mu.Lock()

	cancel := self.cancel
	self.cancel = nil

	reader := self.reader
	self.reader = nil
	self.paged_reader = nil
	self.last_opened = time.Time{}

	self.mu.Unlock()

	// Cancel any future alarms
	if cancel != nil {
		cancel()
	}

	if reader != nil {
		reader.Close()
	}

	return nil
}

func (self *AccessorReader) ReadAt(buf []byte, offset int64) (int, error) {
	self.mu.Lock()

	// It is ok to close the reader at any time. We expect this
	// and just re-open the underlying file when needed.
	if self.reader == nil {
		lifetime := self.Lifetime

		accessor, err := accessors.GetAccessor(self.Accessor, self.Scope)
		if err != nil {
			self.mu.Unlock()
			return 0, err
		}

		reader, err := accessor.OpenWithOSPath(self.File)
		if err != nil {
			self.mu.Unlock()
			return 0, err
		}

		lru_size := self.lru_size
		if lru_size == 0 {
			lru_size = 100
		}

		paged_reader, err := ntfs.NewPagedReader(
			utils.MakeReaderAtter(reader), 1024*8, lru_size)
		if err != nil {
			self.mu.Unlock()
			return 0, err
		}

		// Set an alarm to close the file in the future - this
		// ensures we dont hold open handles for long running
		// queries. Since the paged reader expects the file
		// handles to be closed at any time this is fine - we
		// will just open it again if needed.
		ctx, cancel := context.WithCancel(context.Background())

		// Clear any existing alarms.
		if self.cancel != nil {
			self.cancel()
		}
		self.cancel = cancel

		go func() {
			select {
			// If alarm is cancelled do nothing.
			case <-ctx.Done():
				return

				// Close the file after its lifetime
				// is exhausted.
			case <-time.After(lifetime):
				self.Close()
			}
		}()

		result, err := paged_reader.ReadAt(buf, offset)
		self.paged_reader = paged_reader
		self.reader = reader
		self.last_opened = utils.GetTime().Now()

		self.mu.Unlock()

		// Add ourselves to the active list - this might
		// expire another reader due to the LRU so we release
		// the lock first.
		self.pool.Activate(self)

		return result, err
	}

	paged_reader := self.paged_reader

	// Reading from the paged reader may trigger another reader due to
	// LRU so we release the lock before we do it.
	self.mu.Unlock()

	return paged_reader.ReadAt(buf, offset)
}

func GetReaderPool(scope vfilter.Scope, lru_size int64) *ReaderPool {
	// Manage the reader pool in the scope cache.
	pool_any := vql_subsystem.CacheGet(scope, READERS_CACHE)
	if utils.IsNil(pool_any) {
		if lru_size == 0 {
			lru_size = 100
		}

		// Create a reader pool
		pool := &ReaderPool{
			lru: ttlcache.NewCache(),
		}
		pool.lru.SetCacheSizeLimit(int(lru_size))

		// Close the item on expiration
		pool.lru.SetExpirationReasonCallback(
			func(key string,
				reason ttlcache.EvictionReason, value interface{}) error {
				accessor, ok := value.(*AccessorReader)
				if ok {
					// We may not block this callback so close the
					// accessor in the background.
					go accessor.Close()
				}
				return nil
			})

		// When the item expires from the cache we need to close it.

		vql_subsystem.CacheSet(scope, READERS_CACHE, pool)

		// Destroy the pool when the scope is done.
		_ = vql_subsystem.GetRootScope(scope).AddDestructor(pool.Close)
		return pool
	}

	// There is a pool in the cache, check that it is of the
	// correct type
	pool, ok := pool_any.(*ReaderPool)
	if !ok {
		vql_subsystem.CacheSet(scope, READERS_CACHE, nil)
		return GetReaderPool(scope, lru_size)
	}
	return pool
}

func NewAccessorReader(scope vfilter.Scope,
	accessor string,
	filename *accessors.OSPath,
	lru_size int) (*AccessorReader, error) {

	// Get the reader pool from the scope.
	pool := GetReaderPool(scope, 50)

	// Try to get the reader from the pool
	key := accessor + "://" + filename.String()
	value, err := pool.lru.Get(key)
	if err == nil {
		return value.(*AccessorReader), nil
	}

	accessor_obj, err := accessors.GetAccessor(accessor, scope)
	if err != nil {
		return nil, err
	}

	// If we can figure out the size of the file we might do this now.
	var max_size int64

	// Account for possible case conversions.
	correct_filename := filename
	stat, err := accessor_obj.LstatWithOSPath(filename)
	if err == nil {
		max_size = stat.Size()
		correct_filename = stat.OSPath()
	}

	result := &AccessorReader{
		Accessor: accessor,
		File:     correct_filename,
		key:      key,
		max_size: max_size,
		Scope:    scope,
		pool:     pool,

		// By default close all files after a minute.
		Lifetime: time.Minute,
		lru_size: lru_size,
	}

	_ = pool.lru.Set(key, result)

	return result, nil
}
