package client_info

// Manage storage of all client info records
//
// Since 0.6.8 Velociraptor clients periodically update the client
// records without needing an explicit interrogation step. This allows
// us to be more relaxed about the client info database, since if it
// falls out of date, the client will just update itself at a later
// time anyway.
//
// The ClientInfoManager now maintains an in-memory list of all client
// records. The list is loaded at start time, and is periodically
// flushed to a snapshot. If the server crashes between snapshots, the
// client info can be old, but it will be updated eventually anyway -
// so it is self healing. We are prepared to live with slightly out of
// date information (e.g. ping times, IP addresses and client
// hostnames etc)

// The ClientInfoManager initializes from an on disk store of all
// clients periodically. Currently, only the master node is allowed to
// update this list to avoid needing to co-ordinate locks.

// When the master node writes the snapshot again, the

import (
	"bytes"
	"context"
	"encoding/hex"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/Velocidex/ordereddict"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"google.golang.org/protobuf/proto"
	actions_proto "www.velocidex.com/golang/velociraptor/actions/proto"
	api_proto "www.velocidex.com/golang/velociraptor/api/proto"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	"www.velocidex.com/golang/velociraptor/datastore"
	"www.velocidex.com/golang/velociraptor/file_store"
	"www.velocidex.com/golang/velociraptor/json"
	"www.velocidex.com/golang/velociraptor/logging"
	"www.velocidex.com/golang/velociraptor/paths"
	"www.velocidex.com/golang/velociraptor/result_sets"
	"www.velocidex.com/golang/velociraptor/services"
	"www.velocidex.com/golang/velociraptor/utils"
)

const (
	SYNC_UPDATE = true
)

var (
	info_regex = regexp.MustCompile(`"client_id":"([^"]+)","info":"([^"]+)"`)

	clientInfoDirty = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Name: "client_info_dirty",
		Help: "Is the current client info cache dirty.",
	}, []string{"org"})
)

type Store struct {
	mu   sync.Mutex
	data map[string][]byte

	uuid int64

	dirty bool
}

func (self *Store) Keys() []string {
	self.mu.Lock()
	defer self.mu.Unlock()

	result := make([]string, 0, len(self.data))
	for k := range self.data {
		result = append(result, k)
	}
	return result
}

func (self *Store) Remove(
	config_obj *config_proto.Config, client_id string) {
	self.mu.Lock()
	defer self.mu.Unlock()

	self.dirty = true
	clientInfoDirty.WithLabelValues(config_obj.OrgId).Set(1.0)
	delete(self.data, client_id)
}

func (self *Store) Modify(
	ctx context.Context, config_obj *config_proto.Config, client_id string,
	modifier func(client_info *services.ClientInfo) (
		*services.ClientInfo, error)) error {

	self.mu.Lock()
	defer self.mu.Unlock()

	client_info, _ := self._GetRecord(client_id)
	var record *services.ClientInfo

	if client_info != nil {
		record = &services.ClientInfo{ClientInfo: client_info}
	}

	// If the record was not changed just ignore it.
	new_record, err := modifier(record)
	if err != nil {
		return err
	}

	// Callback can indicate no change is needed by returning a nil
	// for client_info.
	if new_record == nil {
		return nil
	}

	// Write the modified record to the LRU
	return self._SetRecord(config_obj, new_record.ClientInfo)
}

func (self *Store) GetRecord(client_id string) (*actions_proto.ClientInfo, error) {
	self.mu.Lock()
	defer self.mu.Unlock()

	return self._GetRecord(client_id)
}

func (self *Store) _GetRecord(client_id string) (*actions_proto.ClientInfo, error) {
	if client_id == "server" {
		return &actions_proto.ClientInfo{
			ClientId: client_id,
			Hostname: client_id,
			Fqdn:     client_id,
		}, nil
	}

	serialized, pres := self.data[client_id]
	if !pres {
		return nil, utils.NotFoundError
	}

	client_info := &actions_proto.ClientInfo{}
	err := proto.Unmarshal(serialized, client_info)
	if err != nil {
		return nil, err
	}

	// Ensure the client id is populated in the provided record.
	if client_info.ClientId == "" {
		client_info.ClientId = client_id
	}

	return client_info, nil
}

func (self *Store) SetRecord(
	config_obj *config_proto.Config,
	record *actions_proto.ClientInfo) error {
	self.mu.Lock()
	defer self.mu.Unlock()

	return self._SetRecord(config_obj, record)
}

func (self *Store) _SetRecord(
	config_obj *config_proto.Config,
	record *actions_proto.ClientInfo) error {
	serialized, err := proto.Marshal(record)
	if err != nil {
		return err
	}

	self.data[record.ClientId] = serialized
	self.dirty = true
	clientInfoDirty.WithLabelValues(config_obj.OrgId).Set(1.0)
	return nil
}

func (self *ClientInfoManager) LoadFromSnapshot(
	ctx context.Context, config_obj *config_proto.Config) error {
	return self.storage.LoadFromSnapshot(ctx, config_obj)
}

func (self *Store) LoadFromSnapshot(
	ctx context.Context, config_obj *config_proto.Config) error {

	file_store_factory := file_store.GetFileStore(config_obj)
	reader, err := result_sets.NewResultSetReader(
		file_store_factory, paths.CLIENTS_INFO_SNAPSHOT)
	if err != nil || reader.TotalRows() <= 0 {

		// If there is no snapshot file, try to get one from the
		// legacy records.
		return self.LoadSnapshotFromLegacyData(ctx, config_obj)
	}
	defer reader.Close()

	// Clear the snapshot
	self.mu.Lock()
	defer self.mu.Unlock()

	now := time.Now()

	self.data = make(map[string][]byte)
	self.dirty = false
	clientInfoDirty.WithLabelValues(config_obj.OrgId).Set(0.0)

	// Highly optimized reader for speed.
	json_chan, err := reader.JSON(ctx)
	if err != nil {
		return err
	}
	for serialized := range json_chan {
		matches := info_regex.FindStringSubmatch(string(serialized))
		if len(matches) < 3 {
			continue
		}

		client_id := matches[1]
		hex_record := matches[2]

		record, err := hex.DecodeString(hex_record)
		if err != nil {
			continue
		}

		self.data[client_id] = record
	}

	logger := logging.GetLogger(config_obj, &logging.FrontendComponent)
	logger.Info("<green>ClientInfo Manager</> Loaded snapshot in %v for org %v (%v records)",
		time.Now().Sub(now), services.GetOrgName(config_obj), len(self.data))

	return nil
}

// Write the snapshot to storage.
func (self *Store) SaveSnapshot(
	ctx context.Context, config_obj *config_proto.Config, sync bool) error {

	// Only the master can write the snapshot.
	if !services.IsMaster(config_obj) {
		return nil
	}

	now := time.Now()

	// Critical Section....
	self.mu.Lock()

	// Noop - nothing needs to be done if we are not dirty
	if !self.dirty {
		self.mu.Unlock()
		return nil
	}

	// Take a copy of the snapshot to ensure we dont block under lock.

	// Write to memory buffer first then flush to disk in one
	// operation to reduce IO overheads.
	buffer := new(bytes.Buffer)

	for client_id, serialized := range self.data {
		// Use fmt to encode very quickly
		line := fmt.Sprintf("{\"client_id\":%q,\"info\":%q}\n",
			client_id, hex.EncodeToString(serialized))
		buffer.Write([]byte(line))
	}

	self.dirty = false
	clientInfoDirty.WithLabelValues(config_obj.OrgId).Set(0)

	// Total number of records we flush to disk.
	record_count := uint64(len(self.data))

	// Release the lock here as we dont need it for the rest.
	self.mu.Unlock()

	completion := func() {
		journal, err := services.GetJournal(config_obj)
		if err == nil {
			// We do not have to send the update that urgently so it
			// can be async.
			journal.PushRowsToArtifactAsync(ctx, config_obj,
				ordereddict.NewDict().
					Set("From", self.uuid),
				"Server.Internal.ClientInfoSnapshot")
		}

		logger := logging.GetLogger(config_obj, &logging.FrontendComponent)
		logger.Info("<green>ClientInfo Manager</> Written snapshot for org %v in %v (%v records)",
			services.GetOrgName(config_obj), time.Now().Sub(now), record_count)
	}

	// The final write must be synchronous because we need to
	// guarantee it hits the disk
	if sync {
		// For sync writes we dont care about publising snapshot
		// events. These occur during shutdown so it does not matter.
		completion = utils.SyncCompleter
	}

	file_store_factory := file_store.GetFileStore(config_obj)
	writer, err := result_sets.NewResultSetWriter(
		file_store_factory, paths.CLIENTS_INFO_SNAPSHOT,
		json.DefaultEncOpts(),
		completion, result_sets.TruncateMode)
	if err != nil {
		return err
	}
	defer writer.Close()

	writer.WriteJSONL(buffer.Bytes(), record_count)

	return nil
}

// Load data from the legacy client info data.

// In previous versions, client information was stored individually
// for each client in a client record inside the file `<data
// store>/clients/<ClientId>.db`.

// This scheme is inefficient since we need to issue a read IO for
// each client, so the new scheme stores all the client records in a
// single snapshot.

// This function reconstructs the snapshot from the old scheme for
// backwards compatibility.
func (self *Store) LoadSnapshotFromLegacyData(
	ctx context.Context, config_obj *config_proto.Config) error {

	logger := logging.GetLogger(config_obj, &logging.FrontendComponent)
	logger.Info("<green>ClientInfo Manager</> Rebuilding snapshot for org %v from legacy records - this might take a while.",
		services.GetOrgName(config_obj))

	db, err := datastore.GetDB(config_obj)
	if err != nil {
		return err
	}

	children, err := db.ListChildren(config_obj, paths.CLIENTS_ROOT)
	if err != nil {
		return err
	}

	count := 0
	for _, child := range children {

		// On a slow filesystem this can be very slow so we need to be
		// able to interrupt it.
		select {
		case <-ctx.Done():
			return errors.New("Cancelled")

		default:
		}

		if child.IsDir() {
			continue
		}

		client_id := child.Base()
		if !strings.HasPrefix(client_id, "C.") {
			continue
		}

		client_info := &services.ClientInfo{ClientInfo: &actions_proto.ClientInfo{}}
		client_path_manager := paths.NewClientPathManager(client_id)

		// Read the main client record
		err = db.GetSubject(config_obj, client_path_manager.Path(),
			client_info.ClientInfo)
		if err != nil {
			continue
		}

		// Load any labels
		label_record := &api_proto.ClientLabels{}
		err = db.GetSubject(config_obj,
			client_path_manager.Labels(), label_record)
		if err == nil {
			client_info.Labels = label_record.Label
		}

		count++
		if count%1000 == 0 {
			logger.Info("<green>ClientInfo Manager</> Rebuilt %v clients from Legacy data.", count)
		}

		// Now read the ping info in case it is there.
		ping_info := &services.ClientInfo{ClientInfo: &actions_proto.ClientInfo{}}
		err = db.GetSubject(config_obj, client_path_manager.Ping(), ping_info.ClientInfo)
		if err == nil {
			client_info.Ping = ping_info.Ping
			client_info.IpAddress = ping_info.IpAddress
			client_info.LastHuntTimestamp = ping_info.LastHuntTimestamp
			client_info.LastEventTableVersion = ping_info.LastEventTableVersion
		}

		serialized, err := proto.Marshal(client_info)
		if err != nil {
			continue
		}

		self.mu.Lock()
		self.data[client_id] = serialized
		self.dirty = true
		clientInfoDirty.WithLabelValues(config_obj.OrgId).Set(1.0)
		self.mu.Unlock()

	}

	logger.Debug("<green>ClientInfo Manager</> Rebuilt %v clients from Legacy data.", count)

	// Save the data for next time.
	return self.SaveSnapshot(ctx, config_obj, !SYNC_UPDATE)
}

func NewStorage(uuid int64) *Store {
	return &Store{
		data: make(map[string][]byte),
		uuid: uuid,
	}
}
