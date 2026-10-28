package client_info

import (
	"sync"

	"github.com/Velocidex/ordereddict"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	"www.velocidex.com/golang/velociraptor/services"
)

const (
	MAX_MUTATION_UPDATE_SIZE = 10000
)

// Updates to the master node are batched into a mutation object to
// reduce API call overheads. This MutationManager is used to group
// together updates to be flushed periodically to the master.

// Builds up the mutation over time.
type MutationManager struct {
	I_am_master bool

	mu sync.Mutex
	// Only keep the latest times for each client.
	pings                    map[string]uint64
	ip_address               map[string]string
	last_hunt_timestamp      map[string]uint64
	last_event_table_version map[string]uint64
}

func NewMutationManager(config_obj *config_proto.Config) *MutationManager {
	res := &MutationManager{
		I_am_master: services.IsMaster(config_obj),
	}

	res.clear()
	return res
}

func (self *MutationManager) AddPing(client_id string, ping uint64) {
	// The master node does not update the ping info on the minions -
	// they dont really need it.
	if self.I_am_master {
		return
	}

	self.mu.Lock()
	defer self.mu.Unlock()

	self.pings[client_id] = ping
}

func (self *MutationManager) AddIPAddress(client_id string, ip_address string) {
	// The master node does not update the IP Address info on the minions -
	// they dont really need it.
	if self.I_am_master {
		return
	}

	self.mu.Lock()
	defer self.mu.Unlock()

	self.ip_address[client_id] = ip_address
}

func (self *MutationManager) AddLastHuntTimestamp(client_id string, ts uint64) {
	self.mu.Lock()
	defer self.mu.Unlock()

	self.last_hunt_timestamp[client_id] = ts
}

func (self *MutationManager) AddLastEventTableVersion(client_id string, ts uint64) {
	self.mu.Lock()
	defer self.mu.Unlock()

	self.last_event_table_version[client_id] = ts
}

func (self *MutationManager) Size() int {
	self.mu.Lock()
	defer self.mu.Unlock()
	return self.size()
}

func (self *MutationManager) size() int {
	return len(self.pings) + len(self.ip_address) +
		len(self.last_hunt_timestamp) + len(self.last_event_table_version)
}

func (self *MutationManager) clear() {
	// Clear the mutation manager
	self.pings = make(map[string]uint64)
	self.ip_address = make(map[string]string)
	self.last_hunt_timestamp = make(map[string]uint64)
	self.last_event_table_version = make(map[string]uint64)
}

// Limit the size of the update so as not to overflow the gRPC limits.
// The update should be random so it should eventualy update the
// entire mutation pool if needed.
func (self *MutationManager) capDictInt64(in map[string]uint64) *ordereddict.Dict {
	res := ordereddict.NewDict()
	length := len(in)
	if length > MAX_MUTATION_UPDATE_SIZE {
		length = MAX_MUTATION_UPDATE_SIZE
	}

	i := 0
	for k := range in {
		if i > length {
			break
		}
		res.Set(k, int64(in[k]))
		i++
	}

	return res
}

func (self *MutationManager) capDictString(in map[string]string) *ordereddict.Dict {
	res := ordereddict.NewDict()
	length := len(in)
	if length > MAX_MUTATION_UPDATE_SIZE {
		length = MAX_MUTATION_UPDATE_SIZE
	}

	i := 0
	for k := range in {
		if i > length {
			break
		}
		res.Set(k, in[k])
		i++
	}

	return res
}

func (self *MutationManager) GetMutation() (*ordereddict.Dict, int) {
	self.mu.Lock()
	defer self.mu.Unlock()

	size := self.size()

	result := ordereddict.NewDict().
		Set("Ping", self.capDictInt64(self.pings)).
		Set("IpAddress", self.capDictString(self.ip_address)).
		Set("LastHuntTimestamp", self.capDictInt64(self.last_hunt_timestamp)).
		Set("LastEventTableVersion", self.capDictInt64(self.last_event_table_version))

	self.clear()
	return result, size
}
