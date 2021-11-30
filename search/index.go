// Index a client

// Indexing a client helps us to quickly locate the client using a
// search term. For a good index we need to following attributes:

// 1. Quickly recovering a record by searching for an exact term. For
//    example, indexing a client id by label means we need to quickly
//    recover all the client ids that contain the label.

// 2. Approximate prefix match - searching by a prefix efficiently
//    enumerates all clients that are indexed by a term starting with
//    that.

// We index the client using the filesystem - by creating a file
// containing a record, we can retrieve it using the search term. We
// use an index path manager to generate a path where we can store the
// records.

// var index_root IndexPathManager
// record_path := index_root.IndexTerm(term, client_id)
// db.SetSubject(config_obj, record_path, record)

// We can then retrieve the record using the search term:
// record_directories := index_root.EnumerateTerms(term)
// results := ... walk(record_directories)

package search

import (
	"context"
	"errors"
	"strings"
	"sync"
	"time"

	"github.com/google/btree"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	api_proto "www.velocidex.com/golang/velociraptor/api/proto"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	"www.velocidex.com/golang/velociraptor/datastore"
	"www.velocidex.com/golang/velociraptor/file_store/api"
	"www.velocidex.com/golang/velociraptor/logging"
	"www.velocidex.com/golang/velociraptor/paths"
	"www.velocidex.com/golang/velociraptor/services"
)

type SearchOptions int

const (
	// Return the entire client record for matching clients.
	OPTION_CLIENT_RECORDS SearchOptions = 0

	// Return only the matching search terms
	OPTION_NAME_ONLY SearchOptions = 1
)

var (
	stopIteration = errors.New("stopIteration")

	indexer = NewIndexer()

	metricLRUTotalTerms = promauto.NewGauge(
		prometheus.GaugeOpts{
			Name: "search_index_lru_total_terms",
			Help: "LRU for search indexes: Total terms cached",
		})
)

type Record struct {
	*api_proto.IndexRecord
	IndexTerm string
}

func NewRecord(record *api_proto.IndexRecord) Record {
	return Record{
		IndexRecord: record,
		IndexTerm: strings.ToLower(
			record.Term + "/" + record.Entity),
	}
}

func (self Record) Less(than btree.Item) bool {
	than_record := than.(Record)
	return self.IndexTerm < than_record.IndexTerm
}

type Indexer struct {
	mu    sync.Mutex
	btree *btree.BTree
	items int

	ready bool
}

func NewIndexer() *Indexer {
	return &Indexer{
		btree: btree.New(10),
	}
}

func (self *Indexer) Ready() bool {
	self.mu.Lock()
	defer self.mu.Unlock()

	return self.ready
}

func (self *Indexer) Items() int {
	self.mu.Lock()
	defer self.mu.Unlock()

	return self.items
}

func (self *Indexer) AscendGreaterOrEqual(
	record Record, iterator btree.ItemIterator) {
	self.mu.Lock()
	defer self.mu.Unlock()

	self.btree.AscendGreaterOrEqual(record, iterator)
}

func (self *Indexer) Ascend(iterator btree.ItemIterator) {
	self.mu.Lock()
	defer self.mu.Unlock()

	self.btree.Ascend(iterator)
}

func (self *Indexer) Set(record Record) {
	self.mu.Lock()
	defer self.mu.Unlock()

	self.btree.ReplaceOrInsert(record)
	self.items++
	metricLRUTotalTerms.Inc()
}

func (self *Indexer) Delete(record Record) {
	self.mu.Lock()
	defer self.mu.Unlock()

	self.btree.Delete(record)
	self.items--
	metricLRUTotalTerms.Dec()
}

func (self *Indexer) Load(
	ctx context.Context,
	config_obj *config_proto.Config) {

	logger := logging.GetLogger(config_obj, &logging.FrontendComponent)
	logger.Info("<green>Starting</> search index service... Please wait for index to load")

	jobs := make(chan api.DSPathSpec)
	defer close(jobs)

	var wg sync.WaitGroup

	subctx, cancel := context.WithCancel(ctx)
	defer cancel()

	db, err := datastore.GetDB(config_obj)
	if err != nil {
		return
	}

	// Start some workers - this needs to be large enough to avoid
	// deadlock
	var worker func(urn api.DSPathSpec)

	worker = func(urn api.DSPathSpec) {
		defer wg.Done()

		children, _ := db.ListChildren(config_obj, urn)
		for _, child := range children {
			if !child.IsDir() {
				record := &api_proto.IndexRecord{}
				err := db.GetSubject(config_obj, child, record)
				if err != nil {
					continue
				}

				// If it is a client also warm up the client
				// info cache.
				if strings.HasPrefix(record.Entity, "C.") {
					services.GetHostname(record.Entity)

					// Get the full record to warm up all
					// client attributes. If the full record
					// does not exist, then this index entry
					// is stale - just ignore it. This can
					// happen if the client records are
					// removed but the index has not been
					// updated.
					_, err := FastGetApiClient(
						ctx, config_obj, record.Entity)
					if err != nil {
						continue
					}
				}
				indexer.Set(NewRecord(record))
				continue
			}

			// Push another job to a worker
			wg.Add(1)

			select {
			case <-subctx.Done():
				wg.Done()
				return

				// If we can push it to a worker we are done here -
				// move to the next worker.
			case jobs <- child:

				// We can not push to a different worker - i guess we
				// just to it ourselves.
			default:
				worker(child)
			}
		}
	}

	// Start 20 workers
	for i := 0; i < 200; i++ {
		go func() {
			for urn := range jobs {
				worker(urn)
			}
		}()
	}

	go func() {
		for {
			select {
			case <-subctx.Done():
				return

			case <-time.After(time.Second):
				logger.Debug("Loaded %v index entries.", self.Items())
			}
		}
	}()

	// Kick it off at the top level
	now := time.Now()

	wg.Add(1)
	jobs <- paths.CLIENT_INDEX_URN

	wg.Wait()
	logger.Info("<green>Indexing service</> search index loaded %v items in %v",
		self.Items(), time.Now().Sub(now))

	self.mu.Lock()
	self.ready = true
	self.mu.Unlock()
}

// Set the index
func SetIndex(
	config_obj *config_proto.Config, client_id, term string) error {
	path_manager := paths.NewIndexPathManager()
	record := &api_proto.IndexRecord{
		Term:   term,
		Entity: client_id,
	}

	// Set in memory indexer
	indexer.Set(NewRecord(record))

	// An also write to filesystem
	db, err := datastore.GetDB(config_obj)
	if err != nil {
		return err
	}

	path := path_manager.IndexTerm(term, client_id)
	return db.SetSubjectWithCompletion(config_obj, path, record, nil)
}

func UnsetIndex(
	config_obj *config_proto.Config, client_id, term string) error {

	record := &api_proto.IndexRecord{
		Term:   term,
		Entity: client_id,
	}

	// Remove from memory indexer
	indexer.Delete(NewRecord(record))

	// Also remove from file store
	path_manager := paths.NewIndexPathManager()
	db, err := datastore.GetDB(config_obj)
	if err != nil {
		return err
	}

	path := path_manager.IndexTerm(term, client_id)
	_ = db.DeleteSubject(config_obj, path)
	return nil
}

// Returns all the clients that match the term
func SearchIndexWithPrefix(
	ctx context.Context,
	config_obj *config_proto.Config,
	prefix string, options SearchOptions) <-chan *api_proto.IndexRecord {
	output_chan := make(chan *api_proto.IndexRecord)

	prefix = strings.ToLower(prefix)

	go func() {
		defer close(output_chan)

		// Walk the btree and get all prefixes
		indexer.AscendGreaterOrEqual(Record{
			IndexTerm: prefix,
		}, func(i btree.Item) bool {
			record := i.(Record)

			// Detect when we exceeded the prefix constraint to quit
			// early.
			if !strings.HasPrefix(record.IndexTerm, prefix) {
				return false
			}

			select {
			case <-ctx.Done():
				return false

			case output_chan <- record.IndexRecord:
				return true
			}
		})
	}()

	return output_chan
}

// Loads the index lru quickly with many threads.
func LoadIndex(
	ctx context.Context,
	wg *sync.WaitGroup, config_obj *config_proto.Config) {

	// Load the index in the background until we are ready.
	go indexer.Load(ctx, config_obj)
}

func WaitForIndex() {
	for !indexer.Ready() {
		time.Sleep(100 * time.Millisecond)
	}
}
