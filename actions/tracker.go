package actions

import (
	"sync"

	actions_proto "www.velocidex.com/golang/velociraptor/actions/proto"
)

type QueryTracker struct {
	mu sync.Mutex

	queriesToStartRow map[string]uint64
}

func (self *QueryTracker) GetStartRow(query *actions_proto.VQLRequest) uint64 {
	self.mu.Lock()
	defer self.mu.Unlock()

	start_row, _ := self.queriesToStartRow[query.Name]
	return start_row
}

func (self *QueryTracker) AddRows(
	query *actions_proto.VQLRequest, count uint64) {
	self.mu.Lock()
	defer self.mu.Unlock()

	start_row, _ := self.queriesToStartRow[query.Name]
	self.queriesToStartRow[query.Name] = start_row + count
}

func NewQueryTracker() *QueryTracker {
	return &QueryTracker{
		queriesToStartRow: make(map[string]uint64),
	}
}
