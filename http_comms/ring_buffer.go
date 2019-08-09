package http_comms

import (
	"encoding/binary"
	"io"
	"os"
	"sync"

	errors "github.com/pkg/errors"
	"github.com/sirupsen/logrus"
	api_proto "www.velocidex.com/golang/velociraptor/api/proto"
	"www.velocidex.com/golang/velociraptor/logging"
)

const (
	FileMagic         = "VRB\x5e"
	FirstRecordOffset = 50
)

type IRingBuffer interface {
	Enqueue(item []byte)
	AvailableBytes() uint64
	Lease(size uint64) []byte
	Commit()
}

type Header struct {
	ReadPointer    int64
	WritePointer   int64
	MaxSize        int64
	AvailableBytes int64
}

func (self *Header) MarshalBinary() ([]byte, error) {
	data := make([]byte, FirstRecordOffset)
	copy(data, FileMagic)

	binary.LittleEndian.PutUint64(data[4:12], uint64(self.ReadPointer))
	binary.LittleEndian.PutUint64(data[12:20], uint64(self.WritePointer))
	binary.LittleEndian.PutUint64(data[20:28], uint64(self.MaxSize))
	binary.LittleEndian.PutUint64(data[28:36], uint64(self.AvailableBytes))

	return data, nil
}

func (self *Header) UnmarshalBinary(data []byte) error {
	if len(data) < FirstRecordOffset {
		return errors.New("Invalid data")
	}

	if string(data[:4]) != FileMagic {
		return errors.New("Invalid Magic")
	}

	self.ReadPointer = int64(binary.LittleEndian.Uint64(data[4:12]))
	self.WritePointer = int64(binary.LittleEndian.Uint64(data[12:20]))
	self.MaxSize = int64(binary.LittleEndian.Uint64(data[20:28]))
	self.AvailableBytes = int64(binary.LittleEndian.Uint64(data[28:36]))

	return nil
}

type ReadWriterAt interface {
	io.ReaderAt
	io.WriterAt
	Truncate(size int64) error
}

type FileBasedRingBuffer struct {
	config_obj *api_proto.Config

	mu sync.Mutex
	c  *sync.Cond

	fd     ReadWriterAt
	header *Header

	read_buf  []byte
	write_buf []byte

	leased_pointer int64
	leased_bytes   int64
}

func (self *FileBasedRingBuffer) Enqueue(item []byte) {
	self.mu.Lock()
	defer self.mu.Unlock()

	binary.LittleEndian.PutUint64(self.write_buf, uint64(len(item)))
	self.fd.WriteAt(self.write_buf, int64(self.header.WritePointer))
	n, _ := self.fd.WriteAt(item, int64(self.header.WritePointer+8))
	self.header.WritePointer += 8 + int64(n)
	self.header.AvailableBytes += int64(n)

	serialized, _ := self.header.MarshalBinary()
	self.fd.WriteAt(serialized, 0)

	logger := logging.GetLogger(self.config_obj, &logging.ClientComponent)
	logger.WithFields(logrus.Fields{
		"header":        self.header,
		"leased_length": self.leased_pointer,
	}).Info("File Ring Buffer: Enqueue")

	// We need to block here until there is room in the message
	// queue. If the message queue is full, the mutex will be
	// locked and we wait here until the data is pushed through to
	// the server, and enough room is available. This has the
	// effect of blocking the executor and stopping the query
	// until we return.
	for self.header.WritePointer > self.header.MaxSize {
		self.c.Wait()
	}
}

func (self *FileBasedRingBuffer) AvailableBytes() uint64 {
	self.mu.Lock()
	defer self.mu.Unlock()

	return uint64(self.header.AvailableBytes)
}

func (self *FileBasedRingBuffer) Lease(size uint64) []byte {
	self.mu.Lock()
	defer self.mu.Unlock()

	result := []byte{}

	self.leased_bytes = 0

	for self.header.WritePointer > self.leased_pointer {
		n, err := self.fd.ReadAt(self.read_buf, self.leased_pointer)
		if err == nil && n == len(self.read_buf) {
			length := int64(binary.LittleEndian.Uint64(self.read_buf))
			item := make([]byte, length)
			n, err := self.fd.ReadAt(item, self.leased_pointer+8)
			if err == nil && int64(n) == length {
				result = append(result, item...)
			}

			self.leased_pointer += 8 + length
			self.leased_bytes += length

			if uint64(len(result)) > size {
				break
			}
		}
	}

	return result
}

func (self *FileBasedRingBuffer) Commit() {
	self.mu.Lock()
	defer self.mu.Unlock()

	logger := logging.GetLogger(self.config_obj, &logging.ClientComponent)

	// We read up to the write pointer, we may truncate the file now.
	if self.leased_pointer == self.header.WritePointer {
		self.fd.Truncate(0)
		self.header.ReadPointer = FirstRecordOffset
		self.header.WritePointer = FirstRecordOffset
		self.header.AvailableBytes = 0
		self.leased_pointer = FirstRecordOffset

		logger.WithFields(logrus.Fields{
			"header":         self.header,
			"leased_pointer": self.leased_pointer,
		}).Info("File Ring Buffer: Commit / Truncate")

	} else {
		self.header.ReadPointer = self.leased_pointer
		self.header.AvailableBytes -= self.leased_bytes

		serialized, _ := self.header.MarshalBinary()
		self.fd.WriteAt(serialized, 0)

		logger.WithFields(logrus.Fields{
			"header":         self.header,
			"leased_pointer": self.leased_pointer,
		}).Info("File Ring Buffer: Commit")
	}

	self.c.Broadcast()
}

func NewFileBasedRingBuffer(config_obj *api_proto.Config) (*FileBasedRingBuffer, error) {
	filename := os.ExpandEnv(config_obj.Client.LocalBuffer.Filename)
	fd, err := os.OpenFile(filename, os.O_RDWR|os.O_CREATE, 0700)
	if err != nil {
		return nil, err
	}

	header := &Header{
		// Pad the header a bit to allow for extensions.
		WritePointer: FirstRecordOffset,
		ReadPointer:  FirstRecordOffset,
		MaxSize: int64(config_obj.Client.LocalBuffer.DiskSize) +
			FirstRecordOffset,
	}
	data := make([]byte, FirstRecordOffset)
	n, err := fd.ReadAt(data, 0)
	if err == nil {
		err := header.UnmarshalBinary(data[:n])
		// The header is not valid, truncate the file and
		// start again.
		if err != nil {
			err = fd.Truncate(0)
			if err != nil {
				return nil, err
			}
		}
	}

	result := &FileBasedRingBuffer{
		config_obj:     config_obj,
		fd:             fd,
		header:         header,
		read_buf:       make([]byte, 8),
		write_buf:      make([]byte, 8),
		leased_pointer: header.ReadPointer,
	}

	result.c = sync.NewCond(&result.mu)

	logger := logging.GetLogger(config_obj, &logging.ClientComponent)
	logger.WithFields(logrus.Fields{
		"filename": filename,
		"max_size": result.header.MaxSize,
	}).Info("Ring Buffer: Creation")

	return result, nil
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
	self.c.L.Lock()
	defer self.c.L.Unlock()

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
	for self.total_length > self.Size {
		self.c.Wait()
	}
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

func NewLocalBuffer(config_obj *api_proto.Config) IRingBuffer {
	if config_obj.Client.LocalBuffer.DiskSize > 0 &&
		config_obj.Client.LocalBuffer.Filename != "" {
		rb, err := NewFileBasedRingBuffer(config_obj)
		if err == nil {
			return rb
		}
		logger := logging.GetLogger(config_obj, &logging.ClientComponent)
		logger.Error("Unable to create a file based ring buffer - using in memory only.")
	}
	return NewRingBuffer(config_obj)
}
