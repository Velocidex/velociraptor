/* A general purpose cached reader pool

   Can be used by any plugins that wish to return references to an
   open accessor/file set. We maintain an LRU of paged readers so when
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
   scope lifetime so much. Files will eventually get closed and cached
   will be evicted.
*/
package readers

import (
	"context"
	"sync"
	"time"

	ntfs "www.velocidex.com/golang/go-ntfs/parser"
	"www.velocidex.com/golang/velociraptor/glob"
	"www.velocidex.com/golang/velociraptor/third_party/cache"
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
	mu sync.Mutex

	lru *cache.LRUCache
}

// Moves the reader to the head of the LRU.
func (self *ReaderPool) Activate(reader *AccessorReader) {
	self.mu.Lock()
	defer self.mu.Unlock()

	self.lru.Set(reader.Key(), reader)
}

// Flush all contained readers.
func (self *ReaderPool) Close() {
	self.mu.Lock()
	defer self.mu.Unlock()

	for _, k := range self.lru.Keys() {
		self.lru.Delete(k)
	}
}

type AccessorReader struct {
	mu sync.Mutex

	Accessor, File string
	Scope          vfilter.Scope

	key string

	reader       glob.ReadSeekCloser
	paged_reader *ntfs.PagedReader

	created     time.Time
	last_active time.Time

	// Owner pool
	pool *ReaderPool

	// Called to cancel the timed closer.
	cancel func()

	// How long to keep the file handle open
	Lifetime time.Duration
}

func (self *AccessorReader) Size() int {
	return 1
}

func (self *AccessorReader) Key() string {
	return self.key
}
func (self *AccessorReader) Close() {
	self.mu.Lock()
	defer self.mu.Unlock()

	// Cancel any future alarms
	if self.cancel != nil {
		self.cancel()
		self.cancel = nil
	}

	if self.reader != nil {
		self.reader.Close()
		self.reader = nil
		self.paged_reader = nil
	}
}

func (self *AccessorReader) ReadAt(buf []byte, offset int64) (int, error) {
	self.mu.Lock()
	defer self.mu.Unlock()

	// It is ok to close the reader at any time. We expect this
	// and just re-open the underlying file when needed.
	if self.reader == nil {
		accessor, err := glob.GetAccessor(self.Accessor, self.Scope)
		if err != nil {
			return 0, err
		}

		self.reader, err = accessor.Open(self.File)
		if err != nil {
			return 0, err
		}

		self.paged_reader, err = ntfs.NewPagedReader(
			utils.ReaderAtter{self.reader}, 1024*8, 100)
		if err != nil {
			return 0, err
		}
		self.created = time.Now()

		// Add ourselves to the active list - this might
		// expire another reader due to the LRU.
		self.pool.Activate(self)

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
			case <-time.After(self.Lifetime):
				self.Close()
			}
		}()
	}
	self.last_active = time.Now()
	result, err := self.paged_reader.ReadAt(buf, offset)
	return result, err
}

func GetReaderPool(scope vfilter.Scope, lru_size int64) *ReaderPool {
	// Manage the reader pool in the scope cache.
	pool_any := vql_subsystem.CacheGet(scope, READERS_CACHE)
	if utils.IsNil(pool_any) {
		// Create a reader pool
		pool := &ReaderPool{
			lru: cache.NewLRUCache(lru_size),
		}
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

func NewPagedReader(scope vfilter.Scope, accessor, filename string) *AccessorReader {

	// Get the reader pool from the scope.
	pool := GetReaderPool(scope, 50)

	// Try to get the reader from the pool
	key := accessor + "://" + filename
	value, pres := pool.lru.Get(key)
	if pres {
		return value.(*AccessorReader)
	}

	result := &AccessorReader{
		Accessor:    accessor,
		File:        filename,
		key:         key,
		Scope:       scope,
		pool:        pool,
		created:     time.Now(),
		last_active: time.Now(),

		// By default close all files after a minute.
		Lifetime: time.Minute,
	}

	pool.lru.Set(key, result)
	return result
}
