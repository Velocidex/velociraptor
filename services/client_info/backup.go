package client_info

import (
	"context"
	"fmt"
	"sync"

	"google.golang.org/protobuf/proto"
	actions_proto "www.velocidex.com/golang/velociraptor/actions/proto"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	"www.velocidex.com/golang/velociraptor/json"
	"www.velocidex.com/golang/velociraptor/services"
	"www.velocidex.com/golang/velociraptor/utils"
	"www.velocidex.com/golang/vfilter"
)

type ClientInfoBackupProvider struct {
	config_obj *config_proto.Config
	store      *Store
}

func (self ClientInfoBackupProvider) ProviderName() string {
	return "ClientInfoBackupProvider"
}

func (self ClientInfoBackupProvider) Name() []string {
	return []string{"client_info.json"}
}

// The backup will just dump out the contents of the client info manager.
func (self ClientInfoBackupProvider) BackupResults(
	ctx context.Context, wg *sync.WaitGroup,
	container services.BackupContainerWriter) (<-chan vfilter.Row, error) {

	return self.store.BackupResults(ctx, wg)
}

func (self *Store) BackupResults(
	ctx context.Context, wg *sync.WaitGroup) (<-chan vfilter.Row, error) {

	// We dont lock the data so we can take as long as needed.
	clients := self.Keys()
	output := make(chan vfilter.Row)

	wg.Add(1)
	go func() {
		defer wg.Done()
		defer close(output)

		for _, client_id := range clients {
			client_info, err := self.GetRecord(client_id)
			if err != nil {
				continue
			}

			// Convert to JSON for serialization to backups. This is a
			// lot slower than the snapshots but should be more
			// readable in case users want to interoperate with
			// external programs.

			serialized, err := json.MarshalWithOptions(
				client_info, json.DefaultEncOpts())
			if err != nil {
				continue
			}

			record, err := utils.ParseJsonToObject(serialized)
			if err != nil {
				continue
			}

			select {
			case <-ctx.Done():
				return

			case output <- record:
			}
		}
	}()

	return output, nil
}

func (self ClientInfoBackupProvider) Restore(ctx context.Context,
	container services.BackupContainerReader,
	in <-chan vfilter.Row) (stat services.BackupStat, err error) {
	return self.store.Restore(ctx, self.config_obj, in)
}

func (self *Store) Restore(ctx context.Context,
	config_obj *config_proto.Config,
	in <-chan vfilter.Row) (stat services.BackupStat, err error) {

	count := 0
	defer func() {
		// When we are done, we need to reindex all the data. NOTE: We
		// are not holding the lock here because the indexer needs to
		// read the records from the store.
		indexer, err := services.GetIndexer(config_obj)
		if err != nil {
			stat.Error = err
		}
		err = indexer.RebuildIndex(ctx, config_obj)
		if err != nil {
			stat.Error = err
		}
		stat.Message = fmt.Sprintf("Restored %v clients", count)
	}()

	self.mu.Lock()
	defer self.mu.Unlock()

	// Clear the store
	self.data = make(map[string][]byte)

	for {
		select {
		case <-ctx.Done():
			return stat, nil

		case row_any, ok := <-in:
			if !ok {
				return stat, nil
			}

			serialized, err := json.MarshalWithOptions(
				row_any, json.DefaultEncOpts())
			if err != nil {
				continue
			}

			client_info := &actions_proto.ClientInfo{}
			err = json.Unmarshal(serialized, client_info)
			if err != nil {
				continue
			}

			serialized, err = proto.Marshal(client_info)
			if err != nil {
				continue
			}

			count++
			self.data[client_info.ClientId] = serialized
		}
	}
}
