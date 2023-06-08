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
// data information (e.g. ping times, IP addresses and client
// hostnames etc)

import (
	"context"
	"os"
	"sync"

	"github.com/Velocidex/ordereddict"
	actions_proto "www.velocidex.com/golang/velociraptor/actions/proto"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	"www.velocidex.com/golang/velociraptor/file_store"
	"www.velocidex.com/golang/velociraptor/json"
	"www.velocidex.com/golang/velociraptor/paths"
	"www.velocidex.com/golang/velociraptor/result_sets"
	"www.velocidex.com/golang/velociraptor/utils"
)

type Store struct {
	mu   sync.Mutex
	data map[string]string
}

func (self *Store) GetRecord(client_id string) (*actions_proto.ClientInfo, error) {
	self.mu.Lock()
	defer self.mu.Unlock()

	serialized, pres := self.data[client_id]
	if !pres {
		return nil, os.ErrNotExist
	}

	client_info := &actions_proto.ClientInfo{}
	err := json.Unmarshal([]byte(serialized), client_info)
	if err != nil {
		return nil, err
	}
	return client_info, nil
}

func (self *Store) SetRecord(record *actions_proto.ClientInfo) error {
	self.mu.Lock()
	defer self.mu.Unlock()

	serialized, err := json.Marshal(record)
	if err != nil {
		return err
	}

	self.data[record.ClientId] = string(serialized)
	return nil
}

func (self *Store) LoadFromSnapshot(
	ctx context.Context, config_obj *config_proto.Config) error {
	self.mu.Lock()
	defer self.mu.Unlock()

	file_store_factory := file_store.GetFileStore(config_obj)
	reader, err := result_sets.NewResultSetReader(
		file_store_factory, paths.CLIENTS_INFO)
	if err != nil {
		return err
	}
	defer reader.Close()

	for row := range reader.Rows(ctx) {
		client_id, pres := row.GetString("client_id")
		if !pres {
			continue
		}

		record, pres := row.GetString("info")
		if !pres {
			continue
		}

		self.data[client_id] = record
	}

	return nil
}

func (self *Store) SaveSnapshot(
	ctx context.Context, config_obj *config_proto.Config) error {
	self.mu.Lock()
	defer self.mu.Unlock()

	file_store_factory := file_store.GetFileStore(config_obj)
	writer, err := result_sets.NewResultSetWriter(
		file_store_factory, paths.CLIENTS_INFO, json.DefaultEncOpts(),
		utils.BackgroundWriter, result_sets.TruncateMode)
	if err != nil {
		return err
	}
	defer writer.Close()

	for clients_id, serialized := range self.data {
		writer.Write(ordereddict.NewDict().
			Set("clients_id", clients_id).
			Set("info", serialized))
	}

	return nil
}
