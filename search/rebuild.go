package search

import (
	"context"
	"strings"
	"time"

	api_proto "www.velocidex.com/golang/velociraptor/api/proto"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	"www.velocidex.com/golang/velociraptor/datastore"
	"www.velocidex.com/golang/velociraptor/logging"
	"www.velocidex.com/golang/velociraptor/paths"
)

// Load all the client records slowly and rebuild the index. This
// takes a long time. It mirrors the job of the interrogation service
// and so should be kept in sync with it.
func (self *Indexer) LoadIndexFromDatastore(
	ctx context.Context, config_obj *config_proto.Config) error {

	// Building takes a long time so we just build into a temp indexer
	// then swap the btree over.
	indexer := NewIndexer()

	db, err := datastore.GetDB(config_obj)
	if err != nil {
		return err
	}

	children, err := db.ListChildren(config_obj, paths.CLIENTS_ROOT)
	if err != nil {
		return err
	}

	now := time.Now()
	count := 0
	for _, child := range children {
		if child.IsDir() {
			continue
		}

		client_id := child.Base()
		if !strings.HasPrefix(client_id, "C.") {
			continue
		}

		client_info, err := FastGetApiClient(ctx, config_obj, child.Base())
		if err != nil {
			continue
		}

		count++

		// The all item corresponds to the "." search term.
		indexer.Set(NewRecord(&api_proto.IndexRecord{
			Term:   "all",
			Entity: client_id,
		}))

		if client_info.OsInfo.Hostname != "" {
			indexer.Set(NewRecord(&api_proto.IndexRecord{
				Term:   "host:" + client_info.OsInfo.Hostname,
				Entity: client_id,
			}))
		}

		// Add labels to the index.
		for _, label := range client_info.Labels {
			indexer.Set(NewRecord(&api_proto.IndexRecord{
				Term:   "label:" + strings.ToLower(label),
				Entity: client_id,
			}))
		}

		// Add MAC addresses to the index.
		if client_info.OsInfo != nil {
			for _, mac := range client_info.OsInfo.MacAddresses {
				indexer.Set(NewRecord(&api_proto.IndexRecord{
					Entity: client_id,
					Term:   "mac:" + mac,
				}))
			}
		}
	}

	logger := logging.GetLogger(config_obj, &logging.FrontendComponent)
	logger.Info("<green>Indexing service</> search index loaded %v items in %v",
		count, time.Now().Sub(now))

	// Merge the new index quickly
	self.mu.Lock()
	self.ready = true
	self.btree = indexer.btree
	self.dirty = true
	self.mu.Unlock()

	return nil
}
