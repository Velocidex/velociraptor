package http_comms

import (
	"fmt"
	"sync"

	"www.velocidex.com/golang/velociraptor/utils"
)

type Header struct {
	Magic uint64

	ReadPointer  uint64
	WritePointer uint64
	MaxSize      uint64
}

/*
type RingBuffer struct {
	writer io.WriteSeeker
	header Header
}
*/

type RingBuffer struct {
	// We serialize messages into the message queue as they
	// arrive. If the queue is too large we flush it to the
	// server.
	mu       sync.Mutex
	messages [][]byte

	leased_idx    int
	leased_length int

	// Protects total_length
	c            *sync.Cond
	total_length int

	// The maximum size of the ring buffer
	Size int
}

func (self *RingBuffer) Enqueue(item []byte) {
	// We need to block here until there is room in the message
	// queue. If the message queue is being sent to the server,
	// the mutex will be locked and we wait here until the data is
	// pushed through.
	self.c.L.Lock()
	for self.total_length > self.Size {
		self.c.Wait()
	}
	defer self.c.L.Unlock()

	self.messages = append(self.messages, item)
	self.total_length += len(item)

	fmt.Printf("Enquing %v to message_queue %v\n",
		len(item), self.total_length)
}

func (self *RingBuffer) AvailableBytes() int {
	self.mu.Lock()
	defer self.mu.Unlock()

	return self.total_length
}

// Leases a group of messages for transmission. Will not advance the
// read pointer until we know those have been successfully delivered
// via Commit(). This allows us to crash during transmission and we
// will just re-send the messages when we restart.
func (self *RingBuffer) Lease(size int) []byte {
	self.mu.Lock()
	defer self.mu.Unlock()

	self.leased_length = 0
	leased := make([]byte, 0)

	for idx, item := range self.messages {
		leased = append(leased, item...)
		self.leased_length += len(item)
		self.leased_idx = idx
		if self.leased_length > size {
			break
		}
	}

	return leased
}

// Commits by removing the read messages from the ring buffer.
func (self *RingBuffer) Commit() {
	self.mu.Lock()
	defer self.mu.Unlock()

	utils.Debug(self.total_length)
	utils.Debug(self.leased_length)
	utils.Debug(self.leased_idx)

	utils.Debug(len(self.messages))

	if len(self.messages) > self.leased_idx {
		self.messages = self.messages[self.leased_idx+1:]
	}
	self.total_length -= self.leased_length
	self.c.Broadcast()
}

func NewRingBuffer(size int) *RingBuffer {
	result := &RingBuffer{
		messages: make([][]byte, 0),
		Size:     size,
	}
	result.c = sync.NewCond(&result.mu)

	return result
}
