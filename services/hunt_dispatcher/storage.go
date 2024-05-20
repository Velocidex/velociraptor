package hunt_dispatcher

import (
	"context"
	"fmt"
	"os"
	"sync"
	"sync/atomic"
	"time"

	"google.golang.org/protobuf/proto"
	api_proto "www.velocidex.com/golang/velociraptor/api/proto"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	"www.velocidex.com/golang/velociraptor/constants"
	"www.velocidex.com/golang/velociraptor/datastore"
	"www.velocidex.com/golang/velociraptor/file_store"
	"www.velocidex.com/golang/velociraptor/json"
	"www.velocidex.com/golang/velociraptor/logging"
	"www.velocidex.com/golang/velociraptor/paths"
	"www.velocidex.com/golang/velociraptor/result_sets"
	"www.velocidex.com/golang/velociraptor/services"
	"www.velocidex.com/golang/velociraptor/utils"
)

var (
	HuntNotFoundError = utils.Wrap(os.ErrNotExist, "Hunt not found")
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
}

type HuntStorageManagerImpl struct {
	// This is the last timestamp of the latest hunt. At steady
	// state all clients will have run all hunts, therefore we can
	// immediately serve their foreman checks by simply comparing a
	// single number.
	// NOTE: This has to be aligned to 64 bits or 32 bit builds will break
	// https://github.com/golang/go/issues/13868
	last_timestamp uint64

	mu sync.Mutex

	config_obj *config_proto.Config

	hunts map[string]*HuntRecord

	I_am_master bool

	// If any of the hunt objects are dirty this will be set.
	dirty bool

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

func (self *HuntStorageManagerImpl) GetLastTimestamp() uint64 {
	return atomic.LoadUint64(&self.last_timestamp)
}

func (self *HuntStorageManagerImpl) Close(ctx context.Context) {
	atomic.SwapUint64(&self.last_timestamp, 0)
	self.FlushIndex(ctx)
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

	default:
		// Update the hunt object
		hunt_record.dirty = true
		self.dirty = true

		// Update the hunt version
		hunt_record.Version = utils.GetTime().Now().UnixNano()

		// The hunts start time could have been modified - we need to
		// update ours then (and also the metrics).
		if hunt_record.StartTime > self.GetLastTimestamp() {
			dispatcherCurrentTimestamp.Set(
				float64(hunt_record.StartTime))
			atomic.StoreUint64(
				&self.last_timestamp, hunt_record.StartTime)
		}

		hunt_path_manager := paths.NewHuntPathManager(hunt_record.HuntId)
		db, err := datastore.GetDB(self.config_obj)
		if err != nil {
			return services.HuntUnmodified
		}

		err = db.SetSubjectWithCompletion(self.config_obj,
			hunt_path_manager.Path(), hunt_record.Hunt, nil)
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

	if hunt.State == api_proto.Hunt_ARCHIVED {
		delete(self.hunts, hunt.HuntId)
		return db.DeleteSubject(self.config_obj, hunt_path_manager.Path())
	}

	// The hunts start time could have been modified - we need to
	// update ours then (and also the metrics).
	if hunt.StartTime > self.GetLastTimestamp() {
		dispatcherCurrentTimestamp.Set(float64(hunt.StartTime))
		atomic.StoreUint64(&self.last_timestamp, hunt.StartTime)
	}

	self.hunts[hunt.HuntId] = &HuntRecord{
		Hunt:  hunt,
		dirty: true,
	}
	self.dirty = true

	return db.SetSubject(self.config_obj, hunt_path_manager.Path(), hunt)
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
	defer rs_reader.Close()

	err = rs_reader.SeekToRow(offset)
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
			continue
		}

		result = append(result, hunt_obj)
		if int64(len(result)) >= length {
			break
		}
	}

	return result, rs_reader.TotalRows(), nil
}

// Loads hunts from the datastore files. The hunt objects are written
// as discrete files in the data store and this reloads the index from
// those.
func (self *HuntStorageManagerImpl) loadHuntsFromDatastore(
	ctx context.Context, config_obj *config_proto.Config) error {

	// Ensure all the records are ready to read.
	datastore.FlushDatastore(config_obj)

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
		last_timestamp := self.GetLastTimestamp()
		if hunt_obj.StartTime > last_timestamp {
			atomic.StoreUint64(&self.last_timestamp, hunt_obj.StartTime)
			dispatcherCurrentTimestamp.Set(float64(last_timestamp))
		}

		old_hunt_record.Hunt = hunt_obj

		// Hunts read from the old datastore hunt files are marked
		// dirty so they are forced to be written to the index.
		old_hunt_record.dirty = true
		self.dirty = true

		self.hunts[hunt_id] = old_hunt_record
	}

	return nil
}

func (self *HuntStorageManagerImpl) Refresh(
	ctx context.Context, config_obj *config_proto.Config) error {

	err := self.loadHuntsFromDatastore(ctx, config_obj)
	if err != nil {
		return err
	}

	// Create an index file.
	return self.FlushIndex(ctx)
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
