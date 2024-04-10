// Index a client

// Indexing a client helps us to quickly locate the client using a
// search term. For a good index we need the following attributes:

// 1. Quickly recovering a record by searching for an exact term. For
//    example, indexing a client id by label means we need to quickly
//    recover all the client ids that contain the label.

// 2. Approximate prefix match - searching by a prefix efficiently
//    enumerates all clients that are indexed by a term starting with
//    that.

// In previous versions we indexed the client using the
// filesystem. However this proved too slow for networked
// filesystems. We therefore maintain a btree in memory of index
// terms. The index is rebuilt at runtime from the client info manager
// which manage client information using a snapshot

package indexing

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
	"www.velocidex.com/golang/velociraptor/logging"
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
	// The record is stored in the btree and searchable by the index
	// term. The index term should be able to be used on multiple
	// entities hence we add a combination of the record term to the
	// entity to make a unique btree key.
	// E.g Record is {Term: "all", Entity: "C.123"} -> btree key "all/C.123"
	// So searching for all/* will give all clients with term "all".
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

	last_snapshot_read time.Time

	config_obj *config_proto.Config
}

func NewIndexer(config_obj *config_proto.Config) *Indexer {
	return &Indexer{
		btree:      btree.New(10),
		config_obj: config_obj,
	}
}

func (self *Indexer) ItemCount() int {
	self.mu.Lock()
	defer self.mu.Unlock()

	return self.items
}

func (self *Indexer) IsReady() bool {
	self.mu.Lock()
	defer self.mu.Unlock()

	return self.ready
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

func (self *Indexer) Start(
	ctx context.Context, wg *sync.WaitGroup,
	config_obj *config_proto.Config) error {
	return self.RebuildIndex(ctx, config_obj)
}

// Set in memory indexer - it will be flushed later.
func (self *Indexer) SetIndex(client_id, term string) error {
	self.mu.Lock()
	defer self.mu.Unlock()

	return self.setIndex(client_id, term)
}

func (self *Indexer) setIndex(client_id, term string) error {
	record := NewRecord(&api_proto.IndexRecord{
		Term:   term,
		Entity: client_id,
	})

	old := self.btree.ReplaceOrInsert(record)
	if old == nil {
		self.items++
	}
	metricLRUTotalTerms.Inc()
	return nil
}

// Remove from memory indexer
func (self *Indexer) UnsetIndex(client_id, term string) error {
	self.mu.Lock()
	defer self.mu.Unlock()

	record := NewRecord(&api_proto.IndexRecord{
		Term:   term,
		Entity: client_id,
	})

	self.btree.Delete(record)
	self.items--
	metricLRUTotalTerms.Dec()

	return nil
}

// Returns all the clients that match the term
func (self *Indexer) SearchIndexWithPrefix(
	ctx context.Context,
	config_obj *config_proto.Config,
	prefix string) <-chan *api_proto.IndexRecord {
	output_chan := make(chan *api_proto.IndexRecord)

	prefix = strings.ToLower(prefix)

	go func() {
		defer close(output_chan)

		// Take a local copy of all results to avoid having a lock on
		// the search index.
		results := []*Record{}

		// Walk the btree and get all prefixes
		self.AscendGreaterOrEqual(Record{
			IndexTerm: prefix,
		}, func(i btree.Item) bool {
			record := i.(Record)

			// Detect when we exceeded the prefix constraint to quit
			// early.
			if !strings.HasPrefix(record.IndexTerm, prefix) {
				return false
			}

			results = append(results, &record)
			return true
		})

		for _, record := range results {
			select {
			case <-ctx.Done():
				return

			case output_chan <- record.IndexRecord:
			}
		}
	}()

	return output_chan
}

func NewIndexingService(ctx context.Context, wg *sync.WaitGroup,
	config_obj *config_proto.Config) (services.Indexer, error) {

	logger := logging.GetLogger(config_obj, &logging.FrontendComponent)
	logger.Info("<green>Starting</> Indexing Service for %v.",
		services.GetOrgName(config_obj))

	indexer := NewIndexer(config_obj)

	return indexer, indexer.Start(ctx, wg, config_obj)
}
