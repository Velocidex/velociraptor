package indexing

import (
	"context"
	"strings"
	"time"

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

	logger := logging.GetLogger(config_obj, &logging.FrontendComponent)
	logger.Info("<green>Indexing Service</>: Rebuilding Full index from filestore - this can take a while.")

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

		client_info, err := self.FastGetApiClient(ctx, config_obj, child.Base())
		if err != nil {
			continue
		}

		count++

		// The all item corresponds to the "." search term.
		self.SetIndex(client_id, "all")

		if client_info.OsInfo.Hostname != "" {
			self.SetIndex(client_id, "host:"+client_info.OsInfo.Hostname)
		}

		// Add labels to the index.
		for _, label := range client_info.Labels {
			self.SetIndex(client_id, "label:"+strings.ToLower(label))
		}

		// Add MAC addresses to the index.
		if client_info.OsInfo != nil {
			for _, mac := range client_info.OsInfo.MacAddresses {
				self.SetIndex(client_id, "mac:"+mac)
			}
		}
	}

	logger.Info("<green>Indexing service</> search index loaded %v items in %v",
		count, time.Now().Sub(now))

	// Merge the new index quickly and mark ourselves as ready.
	self.mu.Lock()
	self.ready = true
	self.dirty = true
	self.mu.Unlock()

	return nil
}
