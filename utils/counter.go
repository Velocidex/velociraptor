package utils

import (
	"encoding/binary"
	"sync"
	"sync/atomic"

	"github.com/google/uuid"
)

var (
	idx uint64 = uint64(GetGUID() >> 4)

	mock_uuid UUIDGenerator = &RealUUID{}
)

type UUIDGenerator interface {
	GetGUID() int64
}

type RealUUID struct{}

func (self *RealUUID) GetGUID() int64 {
	u := uuid.New()
	return int64(binary.BigEndian.Uint64(u[0:8]) >> 2)
}

type mockUUID struct {
	counter int64
}

func (self *mockUUID) GetGUID() int64 {
	self.counter++
	return self.counter
}

func MockGUID(id int64) func() {
	mock_uuid = &mockUUID{id}
	return func() {
		mock_uuid = &RealUUID{}
	}
}

// Get unique ID
func GetId() uint64 {
	return atomic.AddUint64(&idx, 1)
}

func GetGUID() int64 {
	return mock_uuid.GetGUID()
}

type Counter struct {
	mu    sync.Mutex
	value int
}

func (self *Counter) Inc() {
	self.mu.Lock()
	defer self.mu.Unlock()

	self.value++
}

func (self *Counter) Get() int {
	self.mu.Lock()
	defer self.mu.Unlock()

	return self.value
}
