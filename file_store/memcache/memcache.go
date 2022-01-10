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
)

var (
	metricDataLRU = promauto.NewGauge(
		prometheus.GaugeOpts{
			Name: "memcache_filestore_lru_total",
			Help: "Total files cached in the filestore lru",
		})
)

type MemcacheFileWriter struct {
	mu sync.Mutex

	delegate  *directory.DirectoryFileStore
	filename  api.FSPathSpec
	truncated bool
	buffer    bytes.Buffer
	size      int64

	completion func()
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
func (self *MemcacheFileWriter) Close() error {
	return nil
}

func (self *MemcacheFileWriter) Flush() error {

	// While the file is flushed it blocks other writers to the same
	// file (which will be blocked on the mutex. This ensures writes
	// to the underlying filestore occur in order).
	self.mu.Lock()
	defer self.mu.Unlock()

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

	if self.completion != nil {
		self.completion()
	}

	return err
}

type MemcacheFileStore struct {
	mu sync.Mutex

	config_obj *config_proto.Config
	delegate   *directory.DirectoryFileStore

	data_cache *ttlcache.Cache // map[urn]*MemcacheFileWriter
}

func NewMemcacheFileStore(config_obj *config_proto.Config) *MemcacheFileStore {
	result := &MemcacheFileStore{
		delegate:   directory.NewDirectoryFileStore(config_obj),
		data_cache: ttlcache.NewCache(),
	}

	ttl := config_obj.Datastore.MemcacheWriteMutationMaxAge
	if ttl == 0 {
		ttl = 1
	}
	result.data_cache.SetTTL(time.Duration(ttl) * time.Second)

	result.data_cache.SetNewItemCallback(func(key string, value interface{}) {
		metricDataLRU.Inc()
	})

	result.data_cache.SetExpirationCallback(func(key string, value interface{}) {
		writer, ok := value.(*MemcacheFileWriter)
		if ok {
			writer.Flush()
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
	return self.WriteFileWithCompletion(path, nil)
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
			filename:   path,
			size:       -1,
			completion: completion,
		}
	} else {
		result = result_any.(*MemcacheFileWriter)
		result.completion = completion
	}

	// Always set it so the time can be extended.
	self.data_cache.Set(key, result)

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
