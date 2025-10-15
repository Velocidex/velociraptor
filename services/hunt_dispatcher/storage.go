package hunt_dispatcher

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"sort"
	"sync"
	"sync/atomic"
	"time"

	"google.golang.org/protobuf/proto"
	api_proto "www.velocidex.com/golang/velociraptor/api/proto"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	"www.velocidex.com/golang/velociraptor/constants"
	"www.velocidex.com/golang/velociraptor/datastore"
	"www.velocidex.com/golang/velociraptor/file_store"
	"www.velocidex.com/golang/velociraptor/file_store/api"
	"www.velocidex.com/golang/velociraptor/json"
	"www.velocidex.com/golang/velociraptor/logging"
	"www.velocidex.com/golang/velociraptor/paths"
	"www.velocidex.com/golang/velociraptor/result_sets"
	"www.velocidex.com/golang/velociraptor/services"
	"www.velocidex.com/golang/velociraptor/utils"
)

type HuntIndexEntry struct {
	HuntId      string `json:"HuntId"`
	Description string `json:"Description"`
	Created     uint64 `json:"Created"`
	Started     uint64 `json:"Started"`
	Expires     uint64 `json:"Expires"`
	Creator     string `json:"Creator"`

	// The hunt object is serialized into JSON here to make it quicker
	// to write the index if nothing is changed.
	Hunt []byte `json:"Hunt"`
	Tags string `json:"Tags"`
}

type HuntStorageManager interface {
	// NOTE: This function is very slow! It should only be used by the GUI!
	ListHunts(
		ctx context.Context,
		options result_sets.ResultSetOptions,
		offset int64, length int64) ([]*api_proto.Hunt, int64, error)

	ApplyFuncOnHunts(
		ctx context.Context, options services.HuntSearchOptions,
		cb func(hunt *api_proto.Hunt) error) (res_error error)

	// Gets a copy of the hunt object
	GetHunt(ctx context.Context,
		hunt_id string) (*api_proto.Hunt, error)

	SetHunt(ctx context.Context, hunt *api_proto.Hunt) error

	// Remove the hunt from the local storage. On Minion we only
	// remove from the local memory cache.
	DeleteHunt(ctx context.Context, hunt_id string) error

	ModifyHuntObject(
		ctx context.Context,
		hunt_id string,
		cb func(hunt *HuntRecord) services.HuntModificationAction) services.HuntModificationAction

	Refresh(ctx context.Context,
		config_obj *config_proto.Config) error

	FlushIndex(ctx context.Context) error

	// Get the latest hunt timestamp
	GetLastTimestamp() uint64

	Close(ctx context.Context)

	GetTags(ctx context.Context) []string
}

type HuntStorageManagerImpl struct {
	// This is the last timestamp of the latest hunt. At steady
	// state all clients will have run all hunts, therefore we can
	// immediately serve their foreman checks by simply comparing a
	// single number.
	// NOTE: This has to be aligned to 64 bits or 32 bit builds will break
	// https://github.com/golang/go/issues/13868
	last_timestamp uint64
	closed         int64

	mu sync.Mutex

	config_obj *config_proto.Config

	hunts map[string]*HuntRecord

	// The last time the hunts map was updated.
	last_update time.Time

	I_am_master bool

	// If any of the hunt objects are dirty this will be set.
	dirty bool

	// The last time the index was flushed.
	last_flush_time time.Time
}

func NewHuntStorageManagerImpl(
	config_obj *config_proto.Config) HuntStorageManager {
	result := &HuntStorageManagerImpl{
		config_obj:  config_obj,
		hunts:       make(map[string]*HuntRecord),
		I_am_master: services.IsMaster(config_obj),
	}

	if result.I_am_master {
		backup_service, err := services.GetBackupService(config_obj)
		if err == nil {
			backup_service.Register(&HuntBackupProvider{
				config_obj: config_obj,
				store:      result,
			})
		}
	}

	return result
}

func (self *HuntStorageManagerImpl) MaybeUpdateTimestamp(t uint64) {
	self.mu.Lock()
	defer self.mu.Unlock()
	self._MaybeUpdateTimestamp(t)
}

func (self *HuntStorageManagerImpl) _MaybeUpdateTimestamp(t uint64) {
	last_ts := atomic.LoadUint64(&self.last_timestamp)
	if last_ts < t {
		dispatcherCurrentTimestamp.Set(float64(t))
		atomic.SwapUint64(&self.last_timestamp, t)
	}
}

func (self *HuntStorageManagerImpl) GetLastTimestamp() uint64 {
	return atomic.LoadUint64(&self.last_timestamp)
}

func (self *HuntStorageManagerImpl) Close(ctx context.Context) {
	atomic.SwapUint64(&self.last_timestamp, 0)
	atomic.SwapInt64(&self.closed, 1)
	err := self.FlushIndex(ctx)
	if err != nil {
		logger := logging.GetLogger(self.config_obj, &logging.FrontendComponent)
		logger.Error("HuntStorageManager FlushIndex %v", err)
	}
}

func (self *HuntStorageManagerImpl) ModifyHuntObject(
	ctx context.Context,
	hunt_id string,
	cb func(hunt *HuntRecord) services.HuntModificationAction,
) services.HuntModificationAction {
	self.mu.Lock()
	defer self.mu.Unlock()

	hunt_record, pres := self.hunts[hunt_id]
	// Hunt does not exist, just ignore it.
	if !pres ||
		hunt_record == nil ||
		hunt_record.Hunt == nil {
		return services.HuntUnmodified
	}

	modification := cb(hunt_record)
	switch modification {
	case services.HuntUnmodified:
		return services.HuntUnmodified

		// Asyncronously write to datastore later but update the in
		// memory record now.
	case services.HuntFlushToDatastoreAsync:
		hunt_record.dirty = true
		self.dirty = true

		// The hunts start time could have been modified.
		self._MaybeUpdateTimestamp(hunt_record.StartTime)

		return services.HuntUnmodified

	default:
		// Update the hunt object
		hunt_record.dirty = true
		self.dirty = true

		// Update the hunt version
		hunt_record.Version = utils.GetTime().Now().UnixNano()

		// The hunts start time could have been modified.
		self._MaybeUpdateTimestamp(hunt_record.StartTime)

		hunt_path_manager := paths.NewHuntPathManager(hunt_record.HuntId)
		db, err := datastore.GetDB(self.config_obj)
		if err != nil {
			return services.HuntUnmodified
		}

		err = db.SetSubjectWithCompletion(self.config_obj,
			hunt_path_manager.Path(), hunt_record.Hunt,
			utils.BackgroundWriter)
		if err != nil {
			logger := logging.GetLogger(self.config_obj, &logging.FrontendComponent)
			logger.Error("Flushing %s to disk: %v", hunt_id, err)
			return services.HuntUnmodified
		}
		return modification
	}
}

func (self *HuntStorageManagerImpl) GetHunt(
	ctx context.Context, hunt_id string) (*api_proto.Hunt, error) {
	self.mu.Lock()
	defer self.mu.Unlock()

	hunt, pres := self.hunts[hunt_id]
	if !pres || hunt == nil {
		return nil, fmt.Errorf("%w: %v", services.HuntNotFoundError, hunt_id)
	}

	return proto.Clone(hunt).(*api_proto.Hunt), nil
}

func (self *HuntStorageManagerImpl) SetHunt(
	ctx context.Context, hunt *api_proto.Hunt) error {
	self.mu.Lock()
	defer self.mu.Unlock()

	if hunt == nil {
		return utils.InvalidArgError
	}

	db, err := datastore.GetDB(self.config_obj)
	if err != nil {
		return err
	}

	hunt_path_manager := paths.NewHuntPathManager(hunt.HuntId)

	// Actually delete the hunt from disk. The main deletion happens
	// in the hunt_manager on the master - the hunt dispatcher just
	// needs to remove it from the local cache.
	if hunt.State == api_proto.Hunt_DELETED {
		delete(self.hunts, hunt.HuntId)
		self.dirty = true
		self.last_update = utils.GetTime().Now()
		return nil
	}

	// The hunts start time could have been modified - we need to
	// update ours then (and also the metrics).
	if hunt.StartTime > self.GetLastTimestamp() {
		dispatcherCurrentTimestamp.Set(float64(hunt.StartTime))
		atomic.StoreUint64(&self.last_timestamp, hunt.StartTime)
	}

	self.last_update = utils.GetTime().Now()
	self.hunts[hunt.HuntId] = &HuntRecord{
		Hunt:  hunt,
		dirty: true,
	}
	self.dirty = true
	self._MaybeUpdateTimestamp(hunt.StartTime)

	return db.SetSubject(self.config_obj, hunt_path_manager.Path(), hunt)
}

func (self *HuntStorageManagerImpl) GetTags(
	ctx context.Context) (res []string) {

	_ = self.ApplyFuncOnHunts(ctx, services.AllHunts,
		func(hunt *api_proto.Hunt) error {
			if hunt != nil {
				res = append(res, hunt.Tags...)
			}

			return nil
		})

	res = utils.DeduplicateStringSlice(res)
	sort.Strings(res)
	return res
}

func (self *HuntStorageManagerImpl) ListHunts(
	ctx context.Context,
	options result_sets.ResultSetOptions,
	offset int64, length int64) ([]*api_proto.Hunt, int64, error) {

	hunt_path_manager := paths.NewHuntPathManager("")
	file_store_factory := file_store.GetFileStore(self.config_obj)
	rs_reader, err := result_sets.NewResultSetReaderWithOptions(
		ctx, self.config_obj, file_store_factory,
		hunt_path_manager.HuntIndex(), options)
	if err != nil {
		return nil, 0, err
	}

	// If the index is too old force it to refresh anyway so we always
	// get fresh results.
	self.mu.Lock()
	if rs_reader.MTime().Before(self.last_update) {
		rs_reader.Close()

		err := self._FlushIndex(ctx)
		if err != nil {
			self.mu.Unlock()
			return nil, 0, err
		}

		// Reopen the index with fresh data.
		rs_reader, err = result_sets.NewResultSetReaderWithOptions(
			ctx, self.config_obj, file_store_factory,
			hunt_path_manager.HuntIndex(), options)
		if err != nil {
			self.mu.Unlock()
			return nil, 0, err
		}
	}
	self.mu.Unlock()

	defer rs_reader.Close()

	err = rs_reader.SeekToRow(offset)
	if errors.Is(err, io.EOF) {
		return nil, 0, nil
	}

	if err != nil {
		return nil, 0, err
	}

	// Highly optimized reader for speed.
	json_chan, err := rs_reader.JSON(ctx)
	if err != nil {
		return nil, 0, err
	}

	result := []*api_proto.Hunt{}
	for serialized := range json_chan {
		summary := &HuntIndexEntry{}
		err = json.Unmarshal(serialized, summary)
		if err != nil {
			continue
		}

		// Get the full record from memory cache
		hunt_obj, err := self.GetHunt(ctx, summary.HuntId)
		if err != nil {
			// Something is wrong! The index is referring to a hunt we
			// dont know about - we should re-flush to sync the index.
			self.mu.Lock()
			self.dirty = true
			self.mu.Unlock()
			continue
		}

		result = append(result, hunt_obj)
		if int64(len(result)) >= length {
			break
		}
	}

	return result, rs_reader.TotalRows(), nil
}

func (self *HuntStorageManagerImpl) LoadHuntsFromIndex(
	ctx context.Context, config_obj *config_proto.Config) error {

	self.mu.Lock()
	defer self.mu.Unlock()

	self.dirty = false
	self.hunts = make(map[string]*HuntRecord)

	hunt_path_manager := paths.NewHuntPathManager("")
	file_store_factory := file_store.GetFileStore(config_obj)
	rs_reader, err := result_sets.NewResultSetReaderWithOptions(
		ctx, config_obj, file_store_factory,
		hunt_path_manager.HuntIndex(), result_sets.ResultSetOptions{})
	if err != nil {
		return err
	}
	defer rs_reader.Close()

	for row := range rs_reader.Rows(ctx) {
		serialized_b64, pres := row.GetString("Hunt")
		if !pres {
			continue
		}

		serialized, err := base64.StdEncoding.DecodeString(serialized_b64)
		if err != nil {
			continue
		}

		hunt_obj := &api_proto.Hunt{}
		err = json.Unmarshal(serialized, hunt_obj)
		if err != nil {
			continue
		}
		self.last_update = utils.GetTime().Now()
		self.hunts[hunt_obj.HuntId] = &HuntRecord{
			Hunt:       hunt_obj,
			serialized: serialized,
			dirty:      false,
		}

		self._MaybeUpdateTimestamp(hunt_obj.StartTime)
	}

	return nil
}

// Loads hunts from the datastore files. The hunt objects are written
// as discrete files in the data store and this reloads the index from
// those.
func (self *HuntStorageManagerImpl) loadHuntsFromDatastore(
	ctx context.Context, config_obj *config_proto.Config) error {

	// Ensure all the records are ready to read.
	err := datastore.FlushDatastore(config_obj)
	if err != nil {
		return err
	}

	// Read all the data again from the data store.
	db, err := datastore.GetDB(config_obj)
	if err != nil {
		return err
	}

	hunt_path_manager := paths.NewHuntPathManager("")
	hunts, err := db.ListChildren(config_obj, hunt_path_manager.HuntDirectory())
	if err != nil {
		return err
	}

	requests := make([]*datastore.MultiGetSubjectRequest, 0, len(hunts))
	for _, hunt_path := range hunts {
		hunt_id := hunt_path.Base()
		if !constants.HuntIdRegex.MatchString(hunt_id) {
			continue
		}

		requests = append(requests, datastore.NewMultiGetSubjectRequest(
			&api_proto.Hunt{}, paths.NewHuntPathManager(hunt_id).Path(), hunt_id))
	}

	err = datastore.MultiGetSubject(config_obj, requests)
	if err != nil {
		return err
	}

	self.mu.Lock()
	defer self.mu.Unlock()

	// Now merge the database entries with the current in memory set.
	for _, request := range requests {
		hunt_id := request.Data.(string)
		message := request.Message()
		hunt_obj, ok := message.(*api_proto.Hunt)
		if !ok {
			continue
		}

		if request.Err != nil || hunt_obj.HuntId != hunt_id {
			continue
		}

		// Ignore archived hunts.
		if hunt_obj.State == api_proto.Hunt_ARCHIVED {
			continue
		}

		old_hunt_record, pres := self.hunts[hunt_id]
		if !pres {
			old_hunt_record = &HuntRecord{
				Hunt:  hunt_obj,
				dirty: true,
			}
			self.dirty = true

			// The old hunt record is newer than the one on disk, ignore it.
		} else if old_hunt_record.Version >= hunt_obj.Version {
			continue
		}

		// Maintain the last timestamp as the latest hunt start time.
		self._MaybeUpdateTimestamp(hunt_obj.StartTime)

		old_hunt_record.Hunt = hunt_obj

		// Hunts read from the old datastore hunt files are marked
		// dirty so they are forced to be written to the index.
		old_hunt_record.dirty = true
		self.dirty = true

		self.hunts[hunt_id] = old_hunt_record
		self.last_update = utils.GetTime().Now()
	}

	return nil
}

// Refreshes the in memory hunt objects from the data store.
func (self *HuntStorageManagerImpl) Refresh(
	ctx context.Context, config_obj *config_proto.Config) error {

	// The master will load from the raw db files.
	if self.I_am_master {
		err := self.loadHuntsFromDatastore(ctx, config_obj)
		if err != nil {
			return err
		}

		// Create a fresh index file with the latest data.
		return self.FlushIndex(ctx)
	}

	// Minion can refresh directly from the index
	return self.LoadHuntsFromIndex(ctx, config_obj)
}

// Applies a callback on all hunts. The callback is not allowed to
// modify the hunts.
func (self *HuntStorageManagerImpl) ApplyFuncOnHunts(
	ctx context.Context, options services.HuntSearchOptions,
	cb func(hunt *api_proto.Hunt) error) (res_error error) {

	self.mu.Lock()
	defer self.mu.Unlock()

	for _, record := range self.hunts {
		if options == services.OnlyRunningHunts &&
			record.State != api_proto.Hunt_RUNNING {
			continue
		}

		err := cb(record.Hunt)
		if err != nil {
			res_error = err
		}
	}

	return res_error
}

func (self *HuntStorageManagerImpl) DeleteHunt(
	ctx context.Context, hunt_id string) error {
	self.mu.Lock()
	// First remove from the local memory cache.
	delete(self.hunts, hunt_id)
	self.last_update = utils.GetTime().Now()
	self.dirty = true
	self.mu.Unlock()

	// On the master we also remove the hunts from disk and flush the
	// index.
	if self.I_am_master {
		hunt_path_manager := paths.NewHuntPathManager(hunt_id)
		db, err := datastore.GetDB(self.config_obj)
		if err != nil {
			return err
		}
		_ = db.DeleteSubject(self.config_obj, hunt_path_manager.Path())

		file_store_factory := file_store.GetFileStore(self.config_obj)
		_ = file_store_factory.Delete(hunt_path_manager.Clients())
		_ = file_store_factory.Delete(hunt_path_manager.Clients().
			SetType(api.PATH_TYPE_FILESTORE_JSON_INDEX))

		_ = file_store_factory.Delete(hunt_path_manager.ClientErrors())
		_ = file_store_factory.Delete(hunt_path_manager.ClientErrors().
			SetType(api.PATH_TYPE_FILESTORE_JSON_INDEX))

		// Delete any notebooks etc.
		_ = datastore.RecursiveDelete(self.config_obj, db,
			hunt_path_manager.HuntDataDirectory().AsDatastorePath())

		_ = api.RecursiveDelete(file_store_factory,
			hunt_path_manager.HuntDataDirectory())

		// Delete downloads (exports)
		_ = api.RecursiveDelete(file_store_factory,
			hunt_path_manager.HuntDownloadsDirectory())

		_ = datastore.RecursiveDelete(self.config_obj, db,
			hunt_path_manager.HuntDownloadsDirectory().AsDatastorePath())

		// Delete hunt index
		_ = datastore.RecursiveDelete(self.config_obj, db,
			hunt_path_manager.HuntParticipationIndexDirectory())

		err = self.FlushIndex(ctx)
		if err != nil {
			return err
		}
	}

	return nil
}
