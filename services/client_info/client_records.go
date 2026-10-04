package client_info

import (
	"sync"

	"google.golang.org/protobuf/proto"
	actions_proto "www.velocidex.com/golang/velociraptor/actions/proto"
	"www.velocidex.com/golang/velociraptor/services"
)

type clientRecord struct {
	mu sync.Mutex

	// To reduce memory footprint we serialize the client info record
	// if it not used for a while.
	serialized         []byte
	serialized_version uint64

	dirty bool

	// If this is null we get it from the serialized string, otherwise
	// it is authoritative and we update the serialized string based
	// on it.
	record         *actions_proto.ClientInfo
	record_version uint64

	owner *Store
}

func (self *Store) newClientRecord(client_info *actions_proto.ClientInfo) *clientRecord {
	return &clientRecord{
		dirty:          true,
		record_version: 1,
		record:         client_info,
		owner:          self,
	}
}

func (self *Store) newClientSerializedRecord(serialized []byte) *clientRecord {
	return &clientRecord{
		dirty:              true,
		serialized_version: 1,
		serialized:         serialized,
		owner:              self,
	}
}

func (self *clientRecord) GetSerialized() []byte {
	self.mu.Lock()
	defer self.mu.Unlock()

	// Considered the authoritative record so we serialize it again.
	if self.record != nil &&
		self.record_version > self.serialized_version {

		serialized, err := proto.Marshal(self.record)
		if err != nil {
			return nil
		}

		// Serialized is now the authoritative.
		self.serialized = serialized
		self.serialized_version = self.record_version + 1

		// By evicting the full record on every snapshot write (by
		// default 5min) we keep memory use down to those clients
		// where were last seen less than 5 min ago.
		self.record = nil
	}

	return self.serialized
}

func (self *clientRecord) IsDirty() bool {
	self.mu.Lock()
	defer self.mu.Unlock()

	return self.dirty
}

func (self *clientRecord) GetRecord() (*actions_proto.ClientInfo, error) {
	self.mu.Lock()
	defer self.mu.Unlock()

	res, err := self._GetRecord()
	if err != nil {
		return nil, err
	}

	// Return a copy to keep the cache pristine.
	return proto.Clone(res).(*actions_proto.ClientInfo), nil
}

func (self *clientRecord) _GetRecord() (*actions_proto.ClientInfo, error) {
	if len(self.serialized) > 0 &&
		self.record_version < self.serialized_version {
		self.record = &actions_proto.ClientInfo{}
		err := proto.Unmarshal(self.serialized, self.record)
		if err != nil {
			return nil, err
		}
		self.record_version = self.serialized_version
	}

	return self.record, nil
}

func (self *clientRecord) Modify(
	client_id string,
	modifier func(client_info *services.ClientInfo) (
		*services.ClientInfo, error)) error {

	self.mu.Lock()
	defer self.mu.Unlock()

	record, err := self._GetRecord()
	if err != nil {
		return err
	}

	var client_info *services.ClientInfo

	if record != nil {
		client_info = &services.ClientInfo{
			ClientInfo: record,
		}
	}

	// If the record was not changed just ignore it.
	new_record, err := modifier(client_info)
	if err != nil {
		return err
	}

	// Callback can indicate no change is needed by returning a nil
	// for client_info.
	if new_record == nil {
		return nil
	}

	// Enforce this invariant.
	new_record.ClientId = client_id

	self.dirty = true
	self.serialized = nil
	self.record = new_record.ClientInfo
	self.record_version++

	// Mark the store dirty - doesn't have to be right away but should
	// happen soon.
	go self.owner.SetDirty()

	return nil
}
