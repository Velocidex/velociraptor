package client_info

import (
	"sync"

	"github.com/Velocidex/ordereddict"
)

// Updates to the master node are batched into a mutation object to
// reduce API call overheads. This MutationManager is used to group
// together updates to be flushed periodically to the master.

// Builds up the mutation over time.
type MutationManager struct {
	mu sync.Mutex
	// Only keep the latest times for each client.
	pings                    *ordereddict.Dict
	ip_address               *ordereddict.Dict
	last_hunt_timestamp      *ordereddict.Dict
	last_event_table_version *ordereddict.Dict
}

func NewMutationManager() *MutationManager {
	return &MutationManager{
		pings:                    ordereddict.NewDict(),
		ip_address:               ordereddict.NewDict(),
		last_hunt_timestamp:      ordereddict.NewDict(),
		last_event_table_version: ordereddict.NewDict(),
	}
}

func (self *MutationManager) AddPing(client_id string, ping uint64) {
	self.mu.Lock()
	defer self.mu.Unlock()

	self.pings.Set(client_id, ping)
}

func (self *MutationManager) AddIPAddress(client_id string, ip_address string) {
	self.mu.Lock()
	defer self.mu.Unlock()

	self.ip_address.Set(client_id, ip_address)
}

func (self *MutationManager) AddLastHuntTimestamp(client_id string, ts uint64) {
	self.mu.Lock()
	defer self.mu.Unlock()

	self.last_hunt_timestamp.Set(client_id, ts)
}

func (self *MutationManager) AddLastEventTableVersion(client_id string, ts uint64) {
	self.mu.Lock()
	defer self.mu.Unlock()

	self.last_event_table_version.Set(client_id, ts)
}

func (self *MutationManager) Size() int {
	self.mu.Lock()
	defer self.mu.Unlock()

	return self.pings.Len() + self.ip_address.Len() +
		self.last_hunt_timestamp.Len() + self.last_event_table_version.Len()

}

func (self *MutationManager) GetMutation() *ordereddict.Dict {
	self.mu.Lock()
	defer self.mu.Unlock()

	result := ordereddict.NewDict().
		Set("Ping", self.pings).
		Set("IpAddress", self.ip_address).
		Set("LastHuntTimestamp", self.last_hunt_timestamp).
		Set("LastEventTableVersion", self.last_event_table_version)

	self.pings = ordereddict.NewDict()
	self.ip_address = ordereddict.NewDict()
	self.last_hunt_timestamp = ordereddict.NewDict()
	self.last_event_table_version = ordereddict.NewDict()

	return result
}
