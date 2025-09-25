/*
  This is an experimental file store implementation designed to work
  on very slow filesystem, such as network filesystems.

*/

package memcache

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"sort"
	"sync"
	"sync/atomic"
	"time"

	"github.com/Velocidex/ordereddict"
	"github.com/alitto/pond/v2"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	"www.velocidex.com/golang/velociraptor/file_store/api"
	"www.velocidex.com/golang/velociraptor/file_store/directory"
	"www.velocidex.com/golang/velociraptor/logging"
	"www.velocidex.com/golang/velociraptor/services"
	"www.velocidex.com/golang/velociraptor/services/debug"
	"www.velocidex.com/golang/velociraptor/utils"
)

type FlushOptions bool

var (
	metricDataLRU = promauto.NewGauge(
		prometheus.GaugeOpts{
			Name: "memcache_filestore_lru_total",
			Help: "Total number of writers cached in the filestore lru",
		})

	metricCachedBytes = promauto.NewGauge(
		prometheus.GaugeOpts{
			Name: "memcache_filestore_bytes_total",
			Help: "Total number of bytes waiting to be flushed",
		})

	metricCurrentWriters = promauto.NewGauge(
		prometheus.GaugeOpts{
			Name: "memcache_filestore_current_writers",
			Help: "Total number of current writers flushing (capped at concurrency)",
		})

	metricTotalSyncWrites = promauto.NewCounter(
		prometheus.CounterOpts{
			Name: "memcache_filestore_total_sync_writes",
			Help: "Total number of syncronous writer operations done on the memcache filestore",
		})

	metricTotalWrites = promauto.NewCounter(
		prometheus.CounterOpts{
			Name: "memcache_filestore_total_writes",
			Help: "Total number of writer operations done on the memcache filestore",
		})

	metricTotalDelegateWrites = promauto.NewCounter(
		prometheus.CounterOpts{
			Name: "memcache_filestore_total_delegate_writes",
			Help: "Total number of writer operations done on the delegate filestore",
		})

	metricTotalWritesBytes = promauto.NewCounter(
		prometheus.CounterOpts{
			Name: "memcache_filestore_total_writes_bytes",
			Help: "Total number of bytes writen to the memcache filestore",
		})

	metricTotalDelegateWritesBytes = promauto.NewCounter(
		prometheus.CounterOpts{
			Name: "memcache_filestore_total_delegate_writes_bytes",
			Help: "Total number of bytes writen on the delegate filestore",
		})

	currentlyFlushingError = errors.New("CurrentlyFlushingError")

	currentlyShuttingDownError = errors.New("currentlyShuttingDownError")
)

// The writer is cached in the MemcacheFileStore and manages writing
// on a single file in the filestore.

// The writer writes in two phases - When the Write() method is
// called, data is queued to the internal buffer. When the writer is
// ready to flush the data to its delegate, the buffer is cleared and
// a flush sequence begins. While the delegate write occurs (which can
// take a long time) all writes will continue to be queued to the
// internal buffer. This allows the writer to return immediately, even
// when the delegate is very slow.

type MemcacheFileWriter struct {
	mu sync.Mutex

	// For debugging
	owner      *MemcacheFileStore
	config_obj *config_proto.Config
	id         uint64
	wg         *sync.WaitGroup

	// The delegate will be used to actually flush the data.
	delegate api.FileStore
	key      string
	filename api.FSPathSpec

	// Next flush will truncate the file. All writes will append to
	// the file, so truncate allows the file to be reset to 0 size.
	truncated bool

	// A flag to indicate the Writer is closed and may be reaped from
	// the MemcacheFileStore. We generally try to keep writers around
	// as long as possible to increase the chance of re-opening the
	// same writer.
	closed bool

	// Data will be stored here until being flushed.
	buffer *bytes.Buffer

	written_size  int64
	delegate_size int64

	// Will be true when we are currently flushing.
	flushing bool

	// All writers need to receive a concurrency slot before
	// performing IO - this ensures we do not have too many inflight
	// IOPs at the same time and allows us to control pressure on the
	// delegate filestore.
	concurrency *utils.Concurrency

	// Record the age of the first data in the buffer. If the buffer
	// is older than min_age we start a flush operation.
	last_flush      time.Time
	last_close_time time.Time

	// Keep writes dirty for at least this long to ensure that quick
	// successive writes may be merged. This is the minimum time the
	// writer may be dirty before a flush operation is started
	min_age time.Duration
	max_age time.Duration

	// We keep a list of completions so we can call them all when a
	// file is flushed to disk. We keep the file open for a short time
	// to combine writes to the underlying storage, but if a file is
	// opened, closed then opened again, we need to fire all the
	// completions without losing any.

	// NOTE: We only call completions when the file is closed not for
	// intermediate flush operations which may occur at any time. When
	// performing a flush operation we reset the completions so only
	// the completions that have been actually flushed are called. New
	// completions are associated with the buffer.
	completions []func()
}

func (self *MemcacheFileWriter) Stats() *ordereddict.Dict {
	self.mu.Lock()
	defer self.mu.Unlock()

	now := utils.GetTime().Now()

	last_flush := ""
	if !self.last_flush.IsZero() {
		last_flush = now.Sub(self.last_flush).String()
	}

	last_close_time := ""
	if !self.last_close_time.IsZero() {
		last_close_time = now.Sub(self.last_close_time).String()
	}

	return ordereddict.NewDict().
		Set("Buffered", self.buffer.Len()).
		Set("WrittenSize", self.written_size).
		Set("Closed", self.closed).
		Set("LastFlush", last_flush).
		Set("LastClose", last_close_time).
		Set("CompletionCount", len(self.completions))
}

func (self *MemcacheFileWriter) bufferedSize() int {
	return self.buffer.Len()
}

// Just call the delegate immediately so this update hits the disk.
func (self *MemcacheFileWriter) Update(data []byte, offset int64) error {
	writer, err := self.delegate.WriteFile(self.filename)
	if err != nil {
		return err
	}
	defer writer.Close()

	return writer.Update(data, offset)
}

func (self *MemcacheFileWriter) WriteCompressed(
	data []byte,
	logical_offset uint64,
	uncompressed_size int) (int, error) {
	uncompressed, err := utils.Uncompress(context.Background(), data)
	if err != nil {
		return 0, err
	}

	return self.Write(uncompressed)
}

// Writes go to memory first.
func (self *MemcacheFileWriter) Write(data []byte) (n int, err error) {
	defer api.Instrument("write", "MemcacheFileWriter", nil)()

	// Try to keep our memory use down - Try to flush the store. This
	// has to happen without holding the lock on this writer in case
	// this writer needs to be flushed too.
	defer func() {
		err1 := self.owner.ReduceMemoryToLimit()
		if err1 != nil && err == nil {
			err = err1
		}
	}()

	self.mu.Lock()
	defer self.mu.Unlock()

	if self.last_flush.IsZero() {
		self.last_flush = utils.GetTime().Now()
	}

	self.written_size += int64(len(data))

	self.owner.ChargeBytes(int64(len(data)))
	metricCachedBytes.Add(float64(len(data)))
	metricTotalWrites.Inc()
	metricTotalWritesBytes.Add(float64(len(data)))

	return self.buffer.Write(data)
}

func (self *MemcacheFileWriter) Truncate() error {
	self.mu.Lock()
	defer self.mu.Unlock()

	// Next flush will truncate
	self.truncated = true
	self.buffer.Truncate(0)
	self.written_size = 0
	self.delegate_size = 0

	return nil
}

// Lock free to avoid deadlocks
func (self *MemcacheFileWriter) IsClosed() bool {
	self.mu.Lock()
	defer self.mu.Unlock()
	return self.closed
}

func (self *MemcacheFileWriter) AddCompletion(cb func()) {
	self.mu.Lock()
	defer self.mu.Unlock()

	self.completions = append(self.completions, cb)
}

// Closing the file does not trigger a flush - we just return a
// success status and wait for the file to be written asynchronously.
// We assume no concurrent writes to the same file but closing and
// opening the same path (quicker than cache expiration time) will
// usually give the same writer.
func (self *MemcacheFileWriter) Close() error {
	self.mu.Lock()

	self.closed = true
	self.last_close_time = utils.GetTime().Now()

	// Convert all utils.SyncCompleter calls to sync waits on return
	// from Close(). The writer pool will release us when done.
	wg := sync.WaitGroup{}
	defer wg.Wait()

	sync_call := false
	for idx, c := range self.completions {
		if utils.CompareFuncs(c, utils.SyncCompleter) {
			wg.Add(1)

			self.completions[idx] = wg.Done
			sync_call = true
		}
	}

	// Release the lock before we wait for the flusher.
	self.mu.Unlock()

	// If any of the calls were synchronous do not wait for the usual
	// flush cycle, instead force a flush now and wait for it to
	// complete.
	if sync_call {
		metricTotalSyncWrites.Inc()
		return self.FlushSync()
	}

	// Leave the actual flush for some time in the future and return
	// immediately.
	return nil
}

func (self *MemcacheFileWriter) timeToFlush() bool {
	self.mu.Lock()
	defer self.mu.Unlock()

	// Keep the writer dirty for a short time to ensure we can merge
	// writes.
	return utils.GetTime().Now().Sub(self.last_flush) > self.min_age
}

func (self *MemcacheFileWriter) timeToExpire() bool {
	self.mu.Lock()
	defer self.mu.Unlock()

	// Only expire closed writers
	if !self.closed {
		return false
	}

	// Keep the writer dirty for a short time to ensure we can merge
	// writes.
	return utils.GetTime().Now().Sub(self.last_close_time) > self.max_age
}

func (self *MemcacheFileWriter) callCompletions(completions []func()) {
	for _, c := range completions {
		c()
	}
}

// Begin the flush cycle
func (self *MemcacheFileWriter) Flush() error {
	return self._Flush(true)
}

func (self *MemcacheFileWriter) FlushSync() error {
	return self._Flush(false)
}

func (self *MemcacheFileWriter) _Flush(async bool) error {
	// While the file is flushed it blocks other writers to the same
	// file (which will be blocked on the mutex. This ensures writes
	// to the underlying filestore occur in order).
	self.mu.Lock()
	defer self.mu.Unlock()

	// Copy the completions and buffer so we can serve writes while
	// flushing.
	var completions []func()

	// Only send completions once the file is actually closed. It
	// is possible for the file to flush many times before it is
	// being closed but this does not count as a completion.
	if self.closed {
		completions = append(completions, self.completions...)
		self.completions = nil
	}

	// Nothing to do
	if self.last_flush.IsZero() {
		self.callCompletions(completions)
		return nil
	}

	// Not really an error but we can not flush while we are already
	// flushing - will try again in the next flush cycle.
	if self.flushing {
		self.callCompletions(completions)
		return currentlyFlushingError
	}

	// Will be cleared when the flush is done and we can flush again.
	self.flushing = true

	// Next writes will not truncate since we are truncating now.
	truncated := self.truncated
	self.truncated = false

	// Skip a noop action - nothing was written to this since last
	// time.
	if len(completions) == 0 &&
		!self.closed &&
		!truncated && self.buffer.Len() == 0 {
		self.flushing = false
		return nil
	}

	buffer := self.buffer
	self.buffer = &bytes.Buffer{}
	self.last_flush = time.Time{}

	// Flush in the foreground and wait until the data hits the disk.
	if !async {
		self.mu.Unlock()
		self._FlushSync(buffer.Bytes(), truncated, completions)
		self.mu.Lock()

		self.flushing = false

		return nil
	}

	// Flush in the background and return immediately. We can collect
	// writes into memory in the meantime.
	self.wg.Add(1)
	self.owner.pool.Submit(func() {
		defer self.wg.Done()

		// Not locked! Happens in the background
		self._FlushSync(buffer.Bytes(), truncated, completions)
	})

	return nil
}

func (self *MemcacheFileWriter) Size() (int64, error) {
	self.mu.Lock()
	defer self.mu.Unlock()

	if self.delegate_size < 0 {
		s, err := self.delegate.StatFile(self.filename)
		if err != nil {
			self.delegate_size = 0

		} else {
			self.delegate_size = s.Size()
		}
	}

	return self.delegate_size + self.written_size, nil
}

// Flush the data synchronously. Not locked as we are waiting on a
// concurrency slot here.
func (self *MemcacheFileWriter) _FlushSync(
	data []byte, truncate bool, completions []func()) {

	defer func() {
		// We guarantee to call the completions after this had been
		// flushed but we can not wait for them to exit before we
		// release the writer. Therefore we call these in the
		// background so we can return immediately and release our
		// concurrency slot.
		go func() {
			for _, c := range completions {
				c()
			}
		}()

		// We are ready to flush again!
		self.mu.Lock()
		self.flushing = false
		self.mu.Unlock()
	}()

	// The below is covered by concurrency control - will wait here
	// until there is space for us.
	closer, err := self.concurrency.StartConcurrencyControl(context.Background())
	if err != nil {
		// This can result in data loss if we wait too long for
		// concurrency - but there is not much we can do about it.
		logger := logging.GetLogger(self.config_obj, &logging.FrontendComponent)
		logger.Error("MemcacheFileWriter: Lost data for %v: %v (Maybe increase concurrancy)", self.key, err)
		return
	}
	defer closer()

	metricCurrentWriters.Inc()
	defer metricCurrentWriters.Dec()

	metricTotalDelegateWrites.Inc()
	metricTotalDelegateWritesBytes.Add(float64(len(data)))

	writer, err := self.delegate.WriteFile(self.filename)
	if err != nil {
		logger := logging.GetLogger(self.config_obj, &logging.FrontendComponent)
		logger.Error("MemcacheFileWriter: Lost data for %v: %v", self.key, err)
		return
	}
	defer writer.Close()

	if truncate {
		err := writer.Truncate()
		if err != nil {
			logger := logging.GetLogger(self.config_obj, &logging.FrontendComponent)
			logger.Error("MemcacheFileWriter: Unable to truncare file %v: %v", self.key, err)
			return
		}
	}

	metricCachedBytes.Sub(float64(len(data)))
	_, err = writer.Write(data)
	if err != nil {
		logger := logging.GetLogger(self.config_obj, &logging.FrontendComponent)
		logger.Error("MemcacheFileWriter: Lost data for %v: %v", self.key, err)
	}
	self.owner.ChargeBytes(-int64((len(data))))
}

// Keep all the writers in memory.
type MemcacheFileStore struct {
	// Total number of bytes in flight right now. If this gets too
	// large we start turning writes to be synchronous to push back
	// against writers and protect our memory usage.
	total_cached_bytes int64

	mu sync.Mutex

	// For debugging.
	id         uint64
	ctx        context.Context
	config_obj *config_proto.Config
	wg         *sync.WaitGroup

	// The delegate will be used to actually read the data.
	delegate    api.FileStore
	concurrency *utils.Concurrency

	// Keep the writers in memory - they will be reaped by the reaper thread
	data_cache map[string]*MemcacheFileWriter

	// Minimum time between write and flush - this ensures quick
	// successive writes are merged into larger ones.
	min_age time.Duration

	// Maxmimum time a closed writer will be kept in memory.
	max_age time.Duration

	closed bool

	// Pool of flusher workers
	pool pond.Pool

	target_memory_use int64
}

func NewMemcacheFileStore(
	ctx context.Context,
	config_obj *config_proto.Config) *MemcacheFileStore {

	// Default 5 sec maximum time a closed writer is kept in memory.
	max_age := config_obj.Datastore.MemcacheWriteMutationMaxAge
	if max_age == 0 {
		max_age = 5000
	}

	ttl := config_obj.Datastore.MemcacheWriteMutationMinAge
	if ttl == 0 {
		ttl = 1000
	}

	max_writers := config_obj.Datastore.MemcacheWriteMutationWriters
	if max_writers == 0 {
		max_writers = 200
	}

	target_memory_use := config_obj.Datastore.MemcacheWriteMaxMemory
	if target_memory_use == 0 {
		target_memory_use = 100 * 1024 * 1024
	}

	result := &MemcacheFileStore{
		id:         utils.GetId(),
		ctx:        ctx,
		wg:         &sync.WaitGroup{},
		config_obj: config_obj,
		delegate:   directory.NewDirectoryFileStore(config_obj),
		data_cache: make(map[string]*MemcacheFileWriter),
		concurrency: utils.NewConcurrencyControl(
			int(max_writers), time.Hour),
		max_age:           time.Duration(max_age) * time.Millisecond,
		min_age:           time.Duration(ttl) * time.Millisecond,
		pool:              pond.NewPool(int(max_writers)),
		target_memory_use: target_memory_use,
	}

	// It is very useful to inspect the writer states.
	debug.RegisterProfileWriter(debug.ProfileWriterInfo{
		Name: fmt.Sprintf("memcache_filestore_%v",
			utils.GetOrgId(config_obj)),
		Description:   "Inspect the memcache writer state.",
		ProfileWriter: result.WriteProfile,
		Categories:    []string{"Org", services.GetOrgName(config_obj), "Datastore"},
	})

	go result.Start(ctx)

	return result
}

func (self *MemcacheFileStore) ChargeBytes(count int64) {
	atomic.AddInt64(&self.total_cached_bytes, count)
}

func (self *MemcacheFileStore) FlushCycle(ctx context.Context) {
	writers := []*MemcacheFileWriter{}
	self.mu.Lock()
	for _, w := range self.data_cache {
		writers = append(writers, w)
	}
	self.mu.Unlock()

	for _, w := range writers {
		// Should we flush to disk?
		if w.timeToFlush() {
			w.Flush()
		}

		// Should we remove it?
		if w.timeToExpire() {
			self.mu.Lock()
			delete(self.data_cache, w.key)
			self.mu.Unlock()
			metricDataLRU.Dec()
		}
	}
}

func (self *MemcacheFileStore) Start(ctx context.Context) {
	delay := time.Second
	if self.config_obj.Datastore.MemcacheWriteMutationMaxAge > 0 {
		delay = time.Duration(self.config_obj.Datastore.MemcacheWriteMutationMaxAge) *
			time.Millisecond
	}

	for {
		select {
		case <-ctx.Done():
			return

		case <-utils.GetTime().After(utils.Jitter(delay)):
			self.FlushCycle(ctx)
		}
	}
}

func (self *MemcacheFileStore) ReadFile(
	path api.FSPathSpec) (api.FileReader, error) {
	defer api.Instrument("read_open", "MemcacheFileStore", path)()

	return self.delegate.ReadFile(path)
}

func (self *MemcacheFileStore) WriteFile(
	path api.FSPathSpec) (api.FileWriter, error) {
	return self.WriteFileWithCompletion(path, utils.BackgroundWriter)
}

func (self *MemcacheFileStore) WriteFileWithCompletion(
	path api.FSPathSpec, completion func()) (api.FileWriter, error) {
	defer api.Instrument("write_open", "MemcacheFileStore", path)()

	self.mu.Lock()
	defer self.mu.Unlock()

	// The entire filestore is closed due to shutdown, we dont accept
	// more writers.
	if self.closed {
		return nil, currentlyShuttingDownError
	}

	key := path.AsClientPath()

	var result *MemcacheFileWriter

	result, pres := self.data_cache[key]
	if !pres {
		result = &MemcacheFileWriter{
			owner:         self,
			config_obj:    self.config_obj,
			wg:            self.wg,
			id:            utils.GetId(),
			delegate:      self.delegate,
			key:           key,
			filename:      path,
			buffer:        &bytes.Buffer{},
			concurrency:   self.concurrency,
			min_age:       self.min_age,
			max_age:       self.max_age,
			delegate_size: -1,
			last_flush:    utils.GetTime().Now(),
		}
		self.data_cache[key] = result
		metricDataLRU.Inc()
	}

	// Open the writer again.
	result.mu.Lock()
	result.closed = false
	result.mu.Unlock()

	// Add the completion to the writer.
	if completion != nil {
		result.AddCompletion(completion)
	}

	// Turn the call into syncronous if our memory is exceeded.
	err := self.reduceMemoryToLimit()
	if err != nil {
		return nil, err
	}

	return result, nil
}

func (self *MemcacheFileStore) ReduceMemoryToLimit() error {
	self.mu.Lock()
	defer self.mu.Unlock()

	return self.reduceMemoryToLimit()
}

// Ensure we stay under the memory limit by flushing writers to reduce
// memory use. We block until enough data was released thereby pushing
// back against any writes.
func (self *MemcacheFileStore) reduceMemoryToLimit() error {
	if atomic.LoadInt64(&self.total_cached_bytes) < self.target_memory_use {
		return nil
	}

	// flush the largest caches first.
	sizes := make([]*MemcacheFileWriter, 0, len(self.data_cache))
	for _, v := range self.data_cache {
		sizes = append(sizes, v)
	}

	// To reduce IO we flush larger writers first.
	sort.Slice(sizes, func(i, j int) bool {
		return sizes[i].bufferedSize() > sizes[j].bufferedSize()
	})

	for _, w := range sizes {
		// Flush synchrously while pushing back against our
		// caller. This ensures when we return from here there is
		// enough space to keep writing.
		err := w.FlushSync()
		if err != nil {
			return err
		}

		// As soon as enough space is available, abandon flushing.
		if atomic.LoadInt64(&self.total_cached_bytes) <
			self.target_memory_use {
			return nil
		}
	}

	return nil
}

func (self *MemcacheFileStore) StatFile(path api.FSPathSpec) (api.FileInfo, error) {
	defer api.Instrument("stat", "MemcacheFileStore", path)()

	return self.delegate.StatFile(path)
}

// Clear all the outstanding writers.
func (self *MemcacheFileStore) Flush() {
	self.mu.Lock()
	defer self.mu.Unlock()

	// Stop new writers from being created
	self.closed = true

	// Force all writers to flush now.
	for _, writer := range self.data_cache {
		_ = writer.FlushSync()
	}

	logger := logging.GetLogger(self.config_obj, &logging.FrontendComponent)
	logger.Info("<red>MemcacheFileStore</>: Shutdown: Waiting to flush %v bytes",
		atomic.LoadInt64(&self.total_cached_bytes))

	// Wait for all the flushers to finish
	self.wg.Wait()

	self.data_cache = make(map[string]*MemcacheFileWriter)
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
