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
// terms. We dump the btree into disk periodically called a
// Snapshot. Reading and writing the snapshot is quite fast.

// The master node is responsible for maintaining the snapshot in sync
// - While the snapshot may be read by any node, the master is the
// only node that is allowed to write it.

// It is possible to rebuild the index but for large deplyments this
// is recommended to be done by an external process due to the
// additional load this generates on the master node. The command:

// velociraptor index rebuild

// Will create a timestamped snapshot by rescanning all client records
// in the filestore (for > 100k clients on EFS, this can take a long
// time!)

// The master node will realize a new snapshot is present and reload
// it.

package search

import (
	"context"
	"errors"
	"strings"
	"sync"
	"time"

	"github.com/Velocidex/ordereddict"
	"github.com/google/btree"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	api_proto "www.velocidex.com/golang/velociraptor/api/proto"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	"www.velocidex.com/golang/velociraptor/datastore"
	"www.velocidex.com/golang/velociraptor/file_store"
	"www.velocidex.com/golang/velociraptor/file_store/api"
	"www.velocidex.com/golang/velociraptor/json"
	"www.velocidex.com/golang/velociraptor/logging"
	"www.velocidex.com/golang/velociraptor/paths"
	"www.velocidex.com/golang/velociraptor/result_sets"
	"www.velocidex.com/golang/velociraptor/utils"
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
	dirty bool

	last_snapshot_read time.Time
}

func NewIndexer() *Indexer {
	return &Indexer{
		btree: btree.New(10),
	}
}

// Flush the indexer from memory to disk.
func (self *Indexer) Flush(config_obj *config_proto.Config) error {
	path_manager := paths.NewIndexPathManager()
	dest := path_manager.Snapshot()
	start := time.Now()

	// We need to make sure the snapshot is always valid, so we write
	// to a tmp file and then atomically move to its final place.
	tmp_path_spec := dest.SetType(api.PATH_TYPE_FILESTORE_TMP)
	err := self.WriteSnapshot(config_obj, tmp_path_spec)
	if err != nil {
		return err
	}

	logger := logging.GetLogger(config_obj, &logging.FrontendComponent)
	file_store_factory := file_store.GetFileStore(config_obj)
	err = file_store_factory.Move(tmp_path_spec, dest)
	if err != nil {
		logger.Error("Unable to update snapshot: %v", err)
	} else {
		logger.Debug("Flushed index in %v.", time.Now().Sub(start))
	}

	return nil
}

// Check for newer snapshot files we need to load.
// You can create a new snapshot using "velociraptor index rebuild"
func (self *Indexer) ScanForNewSnapshots(
	ctx context.Context,
	config_obj *config_proto.Config) error {
	path_manager := paths.NewIndexPathManager()
	file_store_factory := file_store.GetFileStore(config_obj)
	children, err := file_store_factory.ListDirectory(
		path_manager.SnapshotDirectory())
	if err != nil {
		return err
	}

	for _, child := range children {
		int_value, ok := utils.ToInt64(child.Name())
		if !ok {
			continue
		}

		timestamp := time.Unix(int_value, 0)
		if timestamp.After(self.last_snapshot_read) {
			// Reload the index.
			logger := logging.GetLogger(config_obj, &logging.FrontendComponent)
			logger.Info("Reloading index snapshot %v",
				child.PathSpec().AsClientPath())
			return self.LoadSnapshot(ctx, config_obj, child.PathSpec())
		}
	}
	return nil
}

func (self *Indexer) WriteSnapshot(
	config_obj *config_proto.Config, dest api.FSPathSpec) error {
	logger := logging.GetLogger(config_obj, &logging.FrontendComponent)
	logger.Info("Writing index on %v\n", dest.AsFilestoreFilename(config_obj))

	// Check if we need to flush the index, if not we can skip it.
	self.mu.Lock()
	defer self.mu.Unlock()

	if !self.dirty {
		return nil
	}
	self.dirty = false

	file_store_factory := file_store.GetFileStore(config_obj)
	rs_writer, err := result_sets.NewResultSetWriter(
		file_store_factory, dest, json.NoEncOpts,
		utils.SyncCompleter, result_sets.TruncateMode)
	if err != nil {
		return err
	}

	// Just write them all down.
	self.btree.Ascend(func(i btree.Item) bool {
		record := i.(Record)
		row := ordereddict.NewDict().
			Set("Entity", record.Entity).
			Set("Term", record.Term)

		rs_writer.Write(row)

		return true
	})

	rs_writer.Close()

	return nil
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

	old := self.btree.ReplaceOrInsert(record)
	if old == nil {
		self.items++
		self.dirty = true
	}
	metricLRUTotalTerms.Inc()
}

func (self *Indexer) Delete(record Record) {
	self.mu.Lock()
	defer self.mu.Unlock()

	self.btree.Delete(record)
	self.items--
	metricLRUTotalTerms.Dec()
}

// A much faster alternative - store all the client index in a single
// file and read it at once.
func (self *Indexer) LoadIndexFromSnapshot(
	ctx context.Context,
	config_obj *config_proto.Config) error {

	path_manager := paths.NewIndexPathManager()
	return self.LoadSnapshot(ctx, config_obj, path_manager.Snapshot())
}

func (self *Indexer) LoadSnapshot(
	ctx context.Context,
	config_obj *config_proto.Config,
	pathspec api.FSPathSpec) error {

	self.last_snapshot_read = time.Now()

	file_store_factory := file_store.GetFileStore(config_obj)
	rs_reader, err := result_sets.NewResultSetReader(
		file_store_factory, pathspec)
	if err != nil {
		return err
	}
	defer rs_reader.Close()

	clients := make(map[string]bool)

	count := 0
	for row := range rs_reader.Rows(ctx) {
		entity, ok := row.GetString("Entity")
		if !ok {
			continue
		}

		// We only actually care about client index entries now.
		if strings.HasPrefix(entity, "C.") {
			clients[entity] = true
		}

		term, ok := row.GetString("Term")
		if !ok {
			continue
		}

		// We should be able to search for the client by client id
		// directly.
		self.Set(NewRecord(&api_proto.IndexRecord{
			Term:   entity,
			Entity: entity,
		}))

		self.Set(NewRecord(&api_proto.IndexRecord{
			Term:   term,
			Entity: entity,
		}))
		count++
	}

	if count == 0 {
		return errors.New("No snapshot")
	}

	logger := logging.GetLogger(config_obj, &logging.FrontendComponent)
	logger.Info("<green>Loaded index from snapshot</> in %v\n",
		time.Now().Sub(self.last_snapshot_read))

	self.mu.Lock()
	self.ready = true
	self.mu.Unlock()

	go func() {
		for c := range clients {
			// Get the full record to warm up all client attributes.
			_, _ = FastGetApiClient(ctx, config_obj, c)
		}
	}()

	return nil
}

func (self *Indexer) Load(
	ctx context.Context,
	config_obj *config_proto.Config) {

	// Loading from the snapshot is very fast, so we do this inline.
	err := self.LoadIndexFromSnapshot(ctx, config_obj)
	if err != nil {
		// If the snapshot is missing, we load from the directory and
		// this can be very slow on EFS so we do it in the background.
		go func() {
			err := self.LoadIndexFromDatastore(ctx, config_obj)
			if err == nil {
				self.Flush(config_obj)
			}
		}()
	}

	logger := logging.GetLogger(config_obj, &logging.FrontendComponent)
	snapshot_wait := 60 * time.Second
	if config_obj.Frontend != nil &&
		config_obj.Frontend.Resources != nil &&
		config_obj.Frontend.Resources.IndexSnapshotFrequency > 0 {
		snapshot_wait = time.Duration(config_obj.Frontend.Resources.
			IndexSnapshotFrequency) * time.Second
	}

	// Check for newer snapshots periodically.
	go func() {
		for {
			err := self.ScanForNewSnapshots(ctx, config_obj)
			if err != nil {
				logger.Error("ScanForNewSnapshots: %v", err)
			}

			select {
			case <-ctx.Done():
				return

			case <-time.After(snapshot_wait):
			}
		}
	}()

}

// Set the index
func SetIndex(
	config_obj *config_proto.Config, client_id, term string) error {
	record := &api_proto.IndexRecord{
		Term:   term,
		Entity: client_id,
	}

	// Set in memory indexer - it will be flushed later.
	indexer.Set(NewRecord(record))

	return nil
}

// Write an index snapshot
func SnapshotIndex(config_obj *config_proto.Config) error {
	return indexer.Flush(config_obj)
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
	prefix string) <-chan *api_proto.IndexRecord {
	output_chan := make(chan *api_proto.IndexRecord)

	prefix = strings.ToLower(prefix)

	go func() {
		defer close(output_chan)

		// Take a local copy of all results to avoid having a lock on
		// the search index.
		results := []*Record{}

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

// Loads the index lru
func LoadIndex(
	ctx context.Context,
	wg *sync.WaitGroup, config_obj *config_proto.Config) {

	indexer.Load(ctx, config_obj)
}

func WaitForIndex() {
	for !indexer.Ready() {
		time.Sleep(100 * time.Millisecond)
	}
}
