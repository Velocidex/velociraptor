package indexing

import (
	"context"
	"errors"
	"strings"
	"time"

	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	"www.velocidex.com/golang/velociraptor/logging"
	"www.velocidex.com/golang/velociraptor/services"
)

// Load all the client records slowly and rebuild the index. This
// takes a long time. It mirrors the job of the interrogation service
// and so should be kept in sync with it.
func (self *Indexer) LoadIndexFromDatastore(
	ctx context.Context, config_obj *config_proto.Config) error {

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
		self.SetIndex(client_id, "all")
		self.SetIndex(client_id, client_id)

		if client_info.Hostname != "" {
			self.SetIndex(client_id, "host:"+client_info.Hostname)
		}

		// Add labels to the index.
		for _, label := range client_info.Labels {
			self.SetIndex(client_id, "label:"+strings.ToLower(label))
		}

		// Add MAC addresses to the index.
		for _, mac := range client_info.MacAddresses {
			self.SetIndex(client_id, "mac:"+mac)
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
