package http_comms

import (
	"sync"

	"github.com/sirupsen/logrus"
	api_proto "www.velocidex.com/golang/velociraptor/api/proto"
	"www.velocidex.com/golang/velociraptor/logging"
)

type Header struct {
	Magic uint64

	ReadPointer  uint64
	WritePointer uint64
	MaxSize      uint64
}

type RingBuffer struct {
	config_obj *api_proto.Config

	// We serialize messages into the messages queue as they
	// arrive.
	mu       sync.Mutex
	messages [][]byte

	leased_idx    uint64
	leased_length uint64

	// Protects total_length
	c            *sync.Cond
	total_length uint64

	// The maximum size of the ring buffer
	Size uint64
}

func (self *RingBuffer) Enqueue(item []byte) {
	// Write the message immediately into the ring buffer. If we
	// crash, the message will be written to disk and
	// retransmitted on restart.
	self.messages = append(self.messages, item)
	self.total_length += uint64(len(item))

	logger := logging.GetLogger(self.config_obj, &logging.ClientComponent)
	logger.WithFields(logrus.Fields{
		"item_len":     len(item),
		"total_length": self.total_length,
	}).Info("Ring Buffer: Enqueue")

	// We need to block here until there is room in the message
	// queue. If the message queue is full, the mutex will be
	// locked and we wait here until the data is pushed through to
	// the server, and enough room is available. This has the
	// effect of blocking the executor and stopping the query
	// until we return.
	self.c.L.Lock()
	for self.total_length > self.Size {
		self.c.Wait()
	}
	defer self.c.L.Unlock()
}

func (self *RingBuffer) AvailableBytes() uint64 {
	self.mu.Lock()
	defer self.mu.Unlock()

	return self.total_length
}

// Leases a group of messages for transmission. Will not advance the
// read pointer until we know those have been successfully delivered
// via Commit(). This allows us to crash during transmission and we
// will just re-send the messages when we restart.
func (self *RingBuffer) Lease(size uint64) []byte {
	self.mu.Lock()
	defer self.mu.Unlock()

	self.leased_length = 0
	leased := make([]byte, 0)

	for idx, item := range self.messages {
		leased = append(leased, item...)
		self.leased_length += uint64(len(item))
		self.leased_idx = uint64(idx)
		if self.leased_length > size {
			break
		}
	}

	return leased
}

func (self *RingBuffer) Rollback() {
	self.mu.Lock()
	defer self.mu.Unlock()

	self.total_length += self.leased_length
	self.leased_length = 0
	self.leased_idx = 0

	logger := logging.GetLogger(self.config_obj, &logging.ClientComponent)
	logger.WithFields(logrus.Fields{
		"total_length":  self.total_length,
		"leased_length": self.leased_length,
	}).Info("Ring Buffer: Rollback")
}

// Commits by removing the read messages from the ring buffer.
func (self *RingBuffer) Commit() {
	self.mu.Lock()
	defer self.mu.Unlock()

	logger := logging.GetLogger(self.config_obj, &logging.ClientComponent)
	logger.WithFields(logrus.Fields{
		"total_length":  self.total_length,
		"leased_length": self.leased_length,
	}).Info("Ring Buffer: Commit")

	if uint64(len(self.messages)) > self.leased_idx {
		self.messages = self.messages[self.leased_idx+1:]
	}
	self.total_length -= self.leased_length
	self.leased_length = 0
	self.leased_idx = 0

	logger.WithFields(logrus.Fields{
		"total_length": self.total_length,
	}).Info("Ring Buffer: Truncate")

	self.c.Broadcast()
}

func NewRingBuffer(config_obj *api_proto.Config) *RingBuffer {
	result := &RingBuffer{
		messages:   make([][]byte, 0),
		Size:       config_obj.Client.LocalBuffer.MemorySize,
		config_obj: config_obj,
	}
	result.c = sync.NewCond(&result.mu)

	return result
}
