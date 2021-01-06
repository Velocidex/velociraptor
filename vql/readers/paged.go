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
	"sync"
	"time"

	ntfs "www.velocidex.com/golang/go-ntfs/parser"
	"www.velocidex.com/golang/velociraptor/glob"
	"www.velocidex.com/golang/velociraptor/third_party/cache"
	"www.velocidex.com/golang/velociraptor/utils"
	vql_subsystem "www.velocidex.com/golang/velociraptor/vql"
	"www.velocidex.com/golang/vfilter"
)

const READERS_CACHE = "$accessor_reader"

type ReaderPool struct {
	mu sync.Mutex

	lru *cache.LRUCache
}

func (self *ReaderPool) Activate(reader *AccessorReader) {
	self.mu.Lock()
	defer self.mu.Unlock()

	self.lru.Set(reader.Key(), reader)
}

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
	Scope          *vfilter.Scope

	key string

	reader       glob.ReadSeekCloser
	paged_reader *ntfs.PagedReader

	created     time.Time
	last_active time.Time

	// Owner pool
	pool *ReaderPool
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

	self.reader.Close()
	self.paged_reader = nil
	self.reader = nil
}

func (self *AccessorReader) ReadAt(buf []byte, offset int64) (int, error) {
	self.mu.Lock()
	defer self.mu.Unlock()

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
			utils.ReaderAtter{self.reader}, 1024, 100)
		if err != nil {
			return 0, err
		}
		self.created = time.Now()

		// Add ourselves to the active list - this might
		// expire another reader due to the LRU.
		self.pool.Activate(self)
	}
	self.last_active = time.Now()
	return self.paged_reader.ReadAt(buf, offset)
}

func NewPagedReader(scope *vfilter.Scope, accessor, filename string) *AccessorReader {
	var pool *ReaderPool

	pool_any := vql_subsystem.CacheGet(scope, READERS_CACHE)
	if utils.IsNil(pool_any) {
		// Create a reader pool
		pool = &ReaderPool{
			lru: cache.NewLRUCache(50),
		}
		vql_subsystem.CacheSet(scope, READERS_CACHE, pool)
		vql_subsystem.GetRootScope(scope).AddDestructor(pool.Close)
	} else {
		pool = pool_any.(*ReaderPool)
	}

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
	}

	pool.lru.Set(key, result)
	return result
}
