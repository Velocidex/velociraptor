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
	"sort"
	"strings"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	api_proto "www.velocidex.com/golang/velociraptor/api/proto"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	"www.velocidex.com/golang/velociraptor/datastore"
	"www.velocidex.com/golang/velociraptor/file_store/api"
	"www.velocidex.com/golang/velociraptor/file_store/path_specs"
	"www.velocidex.com/golang/velociraptor/paths"
	"www.velocidex.com/golang/velociraptor/third_party/cache"
	"www.velocidex.com/golang/velociraptor/utils"
)

type SearchOptions int

const (
	OPTION_ENTITY SearchOptions = 0
	OPTION_KEY    SearchOptions = 1
)

var (
	stopIteration = errors.New("stopIteration")

	// LRU caches ListChildren
	lru = cache.NewLRUCache(10000)

	// Used to mock the clock
	clock utils.Clock = &utils.RealClock{}

	metricLRUHit = promauto.NewCounter(
		prometheus.CounterOpts{
			Name: "search_index_lru_hit",
			Help: "LRU for search indexes",
		})

	metricLRUMiss = promauto.NewCounter(
		prometheus.CounterOpts{
			Name: "search_index_lru_miss",
			Help: "LRU for search indexes",
		})

	metricLRUTotalChildren = promauto.NewGauge(
		prometheus.GaugeOpts{
			Name: "search_index_lru_total_terms",
			Help: "LRU for search indexes: Total terms cached",
		})
)

type lruEntry struct {
	ts       time.Time
	children []api.DSPathSpec
	err      error
}

func (self lruEntry) Size() int {
	return 1
}

func (self lruEntry) Close() {
	metricLRUTotalChildren.Sub(float64(len(self.children)))
}

// Set the index
func SetIndex(
	config_obj *config_proto.Config, client_id, term string) error {
	path_manager := paths.NewIndexPathManager()
	record := &api_proto.IndexRecord{
		Term:   term,
		Entity: client_id,
	}
	db, err := datastore.GetDB(config_obj)
	if err != nil {
		return err
	}

	path := path_manager.IndexTerm(term, client_id)
	invalidateLRU(path)

	return db.SetSubject(config_obj, path, record)
}

func invalidateLRU(path api.DSPathSpec) {
	var tmp api.DSPathSpec = path_specs.NewUnsafeDatastorePath()
	for _, component := range path.Components() {
		tmp = tmp.AddChild(component)
		lru.Delete(getMRUKey(tmp))
	}
}

func UnsetIndex(
	config_obj *config_proto.Config, client_id, term string) error {
	path_manager := paths.NewIndexPathManager()
	db, err := datastore.GetDB(config_obj)
	if err != nil {
		return err
	}

	path := path_manager.IndexTerm(term, client_id)
	invalidateLRU(path)

	return db.DeleteSubject(config_obj, path)
}

// Returns all the clients that match the term
func SearchIndexWithPrefix(
	ctx context.Context,
	config_obj *config_proto.Config,
	prefix string, options SearchOptions) <-chan string {

	output_chan := make(chan string)
	root := paths.CLIENT_INDEX_URN
	partitions := paths.NewIndexPathManager().TermPartitions(prefix)

	db, _ := datastore.GetDB(config_obj)

	go func() {
		defer close(output_chan)

		for client_id := range walkIndexWithPrefix(
			ctx, db, config_obj, root, partitions, options) {
			output_chan <- client_id
		}
	}()

	return output_chan
}

func getMRUKey(path api.DSPathSpec) string {
	return strings.Join(path.Components(), "/")
}

func getChildren(
	config_obj *config_proto.Config,
	root api.DSPathSpec) ([]api.DSPathSpec, error) {
	now := clock.Now()
	key := getMRUKey(root)
	cached_entry_any, pres := lru.Get(key)
	if pres {
		metricLRUHit.Inc()
		cached_entry := cached_entry_any.(*lruEntry)

		// Only use the entry if it is recent enough
		if now.Before(cached_entry.ts.Add(60 * time.Second)) {
			cached_entry.ts = now
			return cached_entry.children, nil
		}

		// Get rid of it and make a new entry
		cached_entry.Close()
	}

	metricLRUMiss.Inc()
	db, _ := datastore.GetDB(config_obj)
	children, err := db.ListChildren(config_obj, root.SetTag("Index"))
	cached_entry := &lruEntry{
		ts:       now,
		children: children,
		err:      err,
	}

	metricLRUTotalChildren.Add(float64(len(children)))
	lru.Set(key, cached_entry)

	return children, err
}

func walkIndexWithPrefix(ctx context.Context,
	db datastore.DataStore,
	config_obj *config_proto.Config,
	root api.DSPathSpec,
	partitions []string, options SearchOptions) chan string {

	output_chan := make(chan string)

	go func() {
		defer close(output_chan)

		children, err := getChildren(config_obj, root)
		if err != nil {
			return
		}

		var next_partitions []string

		// Filter by the partition prefix.
		if len(partitions) > 0 {
			prefix := partitions[0]
			new_children := []api.DSPathSpec{}
			for _, child := range children {
				if strings.HasPrefix(child.Base(), prefix) {
					new_children = append(new_children, child)
				}
			}
			children = new_children

			next_partitions = partitions[1:]
		}

		sort.Slice(children, func(i, j int) bool {
			return children[i].Base() < children[j].Base()
		})

		/*
			fmt.Printf("Listing %v (%v items): %v\n", getMRUKey(root),
				len(children),
				path_specs.DebugPathSpecList(children))

			fmt.Printf("Partitions %v\n", partitions)
		*/

		// First add any non-directories that exist in this directory.
		for _, child := range children {
			var result string

			// For a directory just send an empty string. This will
			// block this goroutine here until someone consumes the
			// string and stop us from listing our directories. If the
			// consumer quits early we are able to avoid listing any
			// directories.
			if !child.IsDir() {
				switch options {
				case OPTION_ENTITY:
					result = child.Base()

				case OPTION_KEY:
					record := &api_proto.IndexRecord{}
					err = db.GetSubject(config_obj, child, record)
					if err != nil {
						continue
					}

					// Return the term
					result = record.Term
				}
			}

			select {
			case <-ctx.Done():
				return

			case output_chan <- result:
			}
		}

		// Now descend the directories.
		child_chans := []chan string{}

		// Spawn workers in parallel to read all child directories.
		for _, child := range children {
			if child.IsDir() {
				child_chans = append(child_chans, walkIndexWithPrefix(
					ctx, db, config_obj, child, next_partitions, options))
			}
		}

		// Push out child directories first - depth first search
		for _, child_chan := range child_chans {
			for client_id := range child_chan {
				select {
				case <-ctx.Done():
					return

				case output_chan <- client_id:
				}
			}
		}

	}()

	return output_chan
}

// Used for testing.
func ResetLRU() {
	lru.Clear()
}

func SetLRUClock(new_clock utils.Clock) {
	clock = new_clock
}

func LRUStats() cache.Stats {
	return lru.Stats()
}
