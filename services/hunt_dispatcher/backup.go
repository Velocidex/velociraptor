package hunt_dispatcher

import (
	"context"
	"fmt"
	"sync"

	api_proto "www.velocidex.com/golang/velociraptor/api/proto"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	"www.velocidex.com/golang/velociraptor/datastore"
	"www.velocidex.com/golang/velociraptor/json"
	"www.velocidex.com/golang/velociraptor/paths"
	"www.velocidex.com/golang/velociraptor/services"
	"www.velocidex.com/golang/velociraptor/utils"
	"www.velocidex.com/golang/vfilter"
)

type HuntBackupProvider struct {
	config_obj *config_proto.Config
	store      *HuntStorageManagerImpl
}

func (self HuntBackupProvider) ProviderName() string {
	return "HuntBackupProvider"
}

func (self HuntBackupProvider) Name() []string {
	return []string{"hunts.json"}
}

// The backup will just dump out the contents of the hunt dispatcher.
func (self HuntBackupProvider) BackupResults(
	ctx context.Context, wg *sync.WaitGroup,
	container services.BackupContainerWriter) (<-chan vfilter.Row, error) {

	return self.store.BackupResults(ctx, wg)
}

func (self *HuntStorageManagerImpl) BackupResults(
	ctx context.Context, wg *sync.WaitGroup) (<-chan vfilter.Row, error) {

	// We dont lock the data so we can take as long as needed.
	self.mu.Lock()
	var hunt_ids []string
	for hunt_id := range self.hunts {
		hunt_ids = append(hunt_ids, hunt_id)
	}
	self.mu.Unlock()

	output := make(chan vfilter.Row)

	wg.Add(1)
	go func() {
		defer wg.Done()
		defer close(output)

		for _, hunt_id := range hunt_ids {
			self.mu.Lock()
			proto_record, pres := self.hunts[hunt_id]
			self.mu.Unlock()

			if !pres {
				continue
			}

			// Convert to JSON for serialization to backups. This is a
			// lot slower than the snapshots but should be more
			// readable in case users want to interoperate with
			// external programs.

			serialized, err := json.MarshalWithOptions(
				proto_record, json.DefaultEncOpts())
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

func (self HuntBackupProvider) Restore(ctx context.Context,
	container services.BackupContainerReader,
	in <-chan vfilter.Row) (stat services.BackupStat, err error) {
	return self.store.Restore(ctx, self.config_obj, in)
}

func (self *HuntStorageManagerImpl) Restore(ctx context.Context,
	config_obj *config_proto.Config,
	in <-chan vfilter.Row) (stat services.BackupStat, err error) {

	count := 0
	defer func() {
		// Force the dispatcher to refresh from the filestore.
		err = self.Refresh(ctx, config_obj)
		if err != nil {
			stat.Error = err
		}
		stat.Message = fmt.Sprintf("Restored %v hunts", count)
	}()

	db, err := datastore.GetDB(config_obj)
	if err != nil {
		return stat, err
	}

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

			hunt_obj := &api_proto.Hunt{}
			err = json.Unmarshal(serialized, hunt_obj)
			if err != nil || hunt_obj.HuntId == "" {
				continue
			}

			count++
			hunt_path_manager := paths.NewHuntPathManager(hunt_obj.HuntId)
			_ = db.SetSubjectWithCompletion(config_obj,
				hunt_path_manager.Path(), hunt_obj,
				utils.BackgroundWriter)
		}
	}
}
