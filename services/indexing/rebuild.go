package indexing

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/google/btree"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	"www.velocidex.com/golang/velociraptor/logging"
	"www.velocidex.com/golang/velociraptor/services"
)

// Load all the client records slowly and rebuild the index. This
// takes a long time. It mirrors the job of the interrogation service
// and so should be kept in sync with it.
func (self *Indexer) RebuildIndex(
	ctx context.Context, config_obj *config_proto.Config) error {

	// Clear the index completely.
	btree := btree.New(10)

	logger := logging.GetLogger(config_obj, &logging.FrontendComponent)
	client_info_manager, err := services.GetClientInfoManager(config_obj)
	if err != nil {
		return err
	}

	now := time.Now()
	count := 0
	for client_id := range client_info_manager.ListClients(ctx) {
		select {
		case <-ctx.Done():
			return errors.New("Cancelled")
		default:
		}

		client_info, err := client_info_manager.Get(ctx, client_id)
		if err != nil {
			continue
		}

		count++

		// The all item corresponds to the "." search term.
		_ = self.setIndexTree(client_id, "all", btree)
		_ = self.setIndexTree(client_id, client_id, btree)

		if client_info.Hostname != "" {
			_ = self.setIndexTree(client_id, "host:"+client_info.Hostname, btree)
		}

		// Add labels to the index.
		for _, label := range client_info.Labels {
			_ = self.setIndexTree(client_id, "label:"+strings.ToLower(label), btree)
		}

		// Add MAC addresses to the index.
		for _, mac := range client_info.MacAddresses {
			_ = self.setIndexTree(client_id, "mac:"+mac, btree)
		}

		// Now add user custom search verbs.
		for k, v := range client_info.Metadata {
			_ = self.setIndexTree(client_id, k+":"+v, btree)
		}
	}

	logger.Debug("<green>Indexing service</> search index loaded %v items in %v",
		count, time.Now().Sub(now))

	// Merge the new index quickly and mark ourselves as ready.
	// Hold a lock on the index while we rebuild it.
	self.mu.Lock()
	defer self.mu.Unlock()

	self.ready = true
	self.btree = btree
	self.items = btree.Len()

	return nil
}
