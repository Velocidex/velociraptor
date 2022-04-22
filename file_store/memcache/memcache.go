/*
  This is an experimental file store implementation designed to work
  on very slow filesystem, such as network filesystems.

*/

package memcache

import (
	"bytes"
	"sync"
	"time"

	"github.com/Velocidex/ttlcache/v2"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	"www.velocidex.com/golang/velociraptor/file_store/api"
	"www.velocidex.com/golang/velociraptor/file_store/directory"
	"www.velocidex.com/golang/velociraptor/utils"
)

var (
	metricDataLRU = promauto.NewGauge(
		prometheus.GaugeOpts{
			Name: "memcache_filestore_lru_total",
			Help: "Total files cached in the filestore lru",
		})

	Clock utils.Clock = utils.RealClock{}
)

type MemcacheFileWriter struct {
	mu sync.Mutex

	delegate  *directory.DirectoryFileStore
	key       string
	filename  api.FSPathSpec
	truncated bool

	// Is the writer currently closed? NOTE!!! There is an implicit
	// assumption that there is only one concurrent writer to the same
	// result set! Writers are all cached in the same data_cache keyed
	// by the same key and are flushed separately.
	closed bool
	buffer bytes.Buffer
	size   int64

	// Writers are kept in cache for memcache_write_mutation_min_age
	// to combine writes. If another write occurs within this time,
	// the cache TTL is extended. However once a write is
	// memcache_write_mutation_max_age old, a flush is
	// forced. Therefore we record the last flush time to determine if
	// a flush should be forced.
	last_flush time.Time

	// We keep a list of completions so we can call them all when a
	// file is flushed to disk. We keep the file open for a short time
	// to combine writes to the underlying storage, but if a file is
	// opened, closed then opened again, we need to fire all the
	// completions without losing any.
	completions []func()
}

func (self *MemcacheFileWriter) Size() (int64, error) {
	self.mu.Lock()
	defer self.mu.Unlock()

	return self._Size()
}

func (self *MemcacheFileWriter) _Size() (int64, error) {
	if self.size >= 0 {
		return self.size, nil
	}

	fs_info, err := self.delegate.StatFile(self.filename)
	if err != nil {
		self.size = 0
		return 0, nil
	}

	self.size = fs_info.Size()
	return self.size, nil
}

func (self *MemcacheFileWriter) Write(data []byte) (int, error) {
	defer api.Instrument("write", "MemcacheFileWriter", nil)()
	self.mu.Lock()
	defer self.mu.Unlock()

	size, err := self._Size()
	if err != nil {
		return 0, err
	}

	self.size = size + int64(len(data))

	return self.buffer.Write(data)
}

func (self *MemcacheFileWriter) Truncate() error {
	self.mu.Lock()
	defer self.mu.Unlock()

	self.truncated = true
	self.buffer.Truncate(0)
	self.size = 0

	return nil
}

// Closing the file does not trigger a flush - we just return a
// success status and wait for the file to be written asynchronously.
// We assume no concurrent writes to the same file but closing and
// opening the same path (quicker than cache expiration time) will
// usually give the same writer.
func (self *MemcacheFileWriter) Close() error {
	self.mu.Lock()
	self.closed = true

	// Convert all utils.SyncCompleter calls to sync waits on return
	// from Close(). The writer pool will release us when done.
	wg := sync.WaitGroup{}
	sync_call := false
	for idx, c := range self.completions {
		if utils.CompareFuncs(c, utils.SyncCompleter) {
			wg.Add(1)

			// Wait for the flusher to close us.
			defer wg.Wait()
			self.completions[idx] = wg.Done
			sync_call = true
		}
	}

	// Release the lock before we wait for the flusher.
	self.mu.Unlock()

	// If any of the calls were synchronous do not wait - just write
	// them now.
	if sync_call {
		return self.Flush()
	}

	return nil
}

func (self *MemcacheFileWriter) Flush() error {

	// While the file is flushed it blocks other writers to the same
	// file (which will be blocked on the mutex. This ensures writes
	// to the underlying filestore occur in order).
	self.mu.Lock()
	defer self.mu.Unlock()

	return self._Flush()
}

func (self *MemcacheFileWriter) _Flush() error {
	defer func() {
		// Only send completions once the file is actually closed. It
		// is possible for the file to flush many times before it is
		// being closed but this does not count as a completion.
		if self.closed {
			for _, c := range self.completions {
				c()
			}
			self.completions = nil
		}
	}()

	self.last_flush = Clock.Now()

	// Skip a noop action.
	if !self.truncated && len(self.buffer.Bytes()) == 0 {
		return nil
	}

	writer, err := self.delegate.WriteFile(self.filename)
	if err != nil {
		return err
	}
	defer writer.Close()

	if self.truncated {
		writer.Truncate()
	}

	_, err = writer.Write(self.buffer.Bytes())

	// Reset the writer for reuse
	self.truncated = false
	self.buffer.Truncate(0)

	return err
}

type MemcacheFileStore struct {
	mu sync.Mutex

	config_obj *config_proto.Config
	delegate   *directory.DirectoryFileStore

	data_cache *ttlcache.Cache // map[urn]*MemcacheFileWriter

	min_age time.Duration
	max_age time.Duration

	closed bool
}

func NewMemcacheFileStore(config_obj *config_proto.Config) *MemcacheFileStore {
	// Default 5 sec maximum write delay time.
	max_age := config_obj.Datastore.MemcacheWriteMutationMaxAge
	if max_age == 0 {
		max_age = 5000
	}

	ttl := config_obj.Datastore.MemcacheWriteMutationMinAge
	if ttl == 0 {
		ttl = 1000
	}

	result := &MemcacheFileStore{
		delegate:   directory.NewDirectoryFileStore(config_obj),
		data_cache: ttlcache.NewCache(),
		max_age:    time.Duration(max_age) * time.Millisecond,
		min_age:    time.Duration(ttl) * time.Millisecond,
	}

	result.data_cache.SetTTL(result.min_age)
	result.data_cache.SkipTTLExtensionOnHit(true)

	result.data_cache.SetNewItemCallback(func(key string, value interface{}) {
		metricDataLRU.Inc()
	})

	result.data_cache.SetExpirationCallback(func(key string, value interface{}) {
		writer, ok := value.(*MemcacheFileWriter)
		if ok {
			writer.mu.Lock()
			defer writer.mu.Unlock()

			writer._Flush()

			// We are not done with it yet - return it to the cache.
			if !result.IsClosed() && !writer.closed {
				result.data_cache.Set(writer.key, writer)
			}
		}

		metricDataLRU.Dec()
	})

	return result
}

func (self *MemcacheFileStore) ReadFile(
	path api.FSPathSpec) (api.FileReader, error) {
	defer api.Instrument("read_open", "MemcacheFileStore", path)()

	return self.delegate.ReadFile(path)
}

func (self *MemcacheFileStore) WriteFile(path api.FSPathSpec) (api.FileWriter, error) {
	return self.WriteFileWithCompletion(path, utils.BackgroundWriter)
}

func (self *MemcacheFileStore) WriteFileWithCompletion(
	path api.FSPathSpec, completion func()) (api.FileWriter, error) {
	defer api.Instrument("write_open", "MemcacheFileStore", path)()

	self.mu.Lock()
	defer self.mu.Unlock()

	key := path.AsClientPath()

	var result *MemcacheFileWriter

	result_any, err := self.data_cache.Get(key)
	if err != nil {
		result = &MemcacheFileWriter{
			delegate:   self.delegate,
			key:        key,
			filename:   path,
			last_flush: Clock.Now(),
			size:       -1,
		}

		// Item is new set it in the cache with default TTL.
		self.data_cache.Set(key, result)

	} else {
		result = result_any.(*MemcacheFileWriter)
		result.closed = false

		// If we have more time until the max_age, re-set it into the
		// cache and extend ttl, otherwise, let it expire normally
		// where it will be flushed.
		if result.last_flush.Add(self.max_age).After(Clock.Now()) {
			self.data_cache.Touch(key)
		}
	}

	// Add the completion to the writer.
	if completion != nil {
		result.completions = append(result.completions, completion)
	}

	return result, nil
}

func (self *MemcacheFileStore) StatFile(path api.FSPathSpec) (api.FileInfo, error) {
	defer api.Instrument("stat", "MemcacheFileStore", path)()

	return self.delegate.StatFile(path)
}

// Clear all the outstanding writers.
func (self *MemcacheFileStore) Flush() {
	self.data_cache.Flush()
}

func (self *MemcacheFileStore) IsClosed() bool {
	self.mu.Lock()
	defer self.mu.Unlock()

	return self.closed
}

func (self *MemcacheFileStore) Close() error {
	self.mu.Lock()
	self.closed = true
	self.mu.Unlock()

	self.Flush()
	return nil
}

func (self *MemcacheFileStore) Move(src, dest api.FSPathSpec) error {
	defer api.Instrument("move", "MemcacheFileStore", src)()

	return self.delegate.Move(src, dest)
}

func (self *MemcacheFileStore) ListDirectory(root_path api.FSPathSpec) ([]api.FileInfo, error) {
	defer api.Instrument("list", "MemcacheFileStore", root_path)()

	return self.delegate.ListDirectory(root_path)
}

func (self *MemcacheFileStore) Delete(path api.FSPathSpec) error {
	defer api.Instrument("delete", "MemoryFileStore", nil)()

	return self.delegate.Delete(path)
}
