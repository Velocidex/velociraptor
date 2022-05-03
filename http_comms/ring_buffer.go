package http_comms

import (
	"context"
	"encoding/binary"
	"io"
	"os"
	"runtime"
	"sync"

	errors "github.com/pkg/errors"
	"github.com/sirupsen/logrus"
	"google.golang.org/protobuf/proto"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	"www.velocidex.com/golang/velociraptor/constants"
	crypto_proto "www.velocidex.com/golang/velociraptor/crypto/proto"
	"www.velocidex.com/golang/velociraptor/executor"
	"www.velocidex.com/golang/velociraptor/json"
	"www.velocidex.com/golang/velociraptor/logging"
	"www.velocidex.com/golang/velociraptor/utils"
)

const (
	FileMagic         = "VRB\x5e"
	FirstRecordOffset = 50
)

type IRingBuffer interface {
	Enqueue(item []byte)

	// How many bytes are currently available to be sent.
	AvailableBytes() uint64

	// Lease this much data from the buffer - the data is not deleted,
	// but it is kept in the file until it is committed.
	Lease(size uint64) []byte

	// The total size of data in the ring buffer - sum of
	// AvailableBytes and LeasedBytes
	TotalSize() uint64

	Commit()
	Reset()
	Close()
}

type Header struct {
	ReadPointer    int64 // Leasing will start at this file offset.
	WritePointer   int64 // Enqueue will write at this file position.
	MaxSize        int64 // Block Enqueue once WritePointer goes past this.
	AvailableBytes int64 // Available to be leased.  Size of data

	// that is currently leased. If the client crashes we replay
	// the leased data again. This should be 0 when we open a
	// file.
	LeasedBytes int64
}

func (self *Header) MarshalBinary() ([]byte, error) {
	data := make([]byte, FirstRecordOffset)
	copy(data, FileMagic)

	binary.LittleEndian.PutUint64(data[4:12], uint64(self.ReadPointer))
	binary.LittleEndian.PutUint64(data[12:20], uint64(self.WritePointer))
	binary.LittleEndian.PutUint64(data[20:28], uint64(self.MaxSize))
	binary.LittleEndian.PutUint64(data[28:36], uint64(self.AvailableBytes))
	binary.LittleEndian.PutUint64(data[36:44], uint64(self.LeasedBytes))

	return data, nil
}

func (self *Header) UnmarshalBinary(data []byte) error {
	if len(data) < FirstRecordOffset {
		return errors.New("Invalid header length")
	}

	if string(data[:4]) != FileMagic {
		return errors.New("Invalid Magic")
	}

	self.ReadPointer = int64(binary.LittleEndian.Uint64(data[4:12]))
	self.WritePointer = int64(binary.LittleEndian.Uint64(data[12:20]))
	self.MaxSize = int64(binary.LittleEndian.Uint64(data[20:28]))
	self.AvailableBytes = int64(binary.LittleEndian.Uint64(data[28:36]))
	self.LeasedBytes = int64(binary.LittleEndian.Uint64(data[36:44]))

	return nil
}

type ReadWriterAt interface {
	io.ReaderAt
	io.WriterAt
	Truncate(size int64) error
}

type FileBasedRingBuffer struct {
	config_obj *config_proto.Config

	mu sync.Mutex
	c  *sync.Cond

	fd     *os.File
	header *Header
	closed bool

	read_buf  []byte
	write_buf []byte

	// The file offset where leases come from.
	leased_pointer int64

	log_ctx *logging.LogContext
}

func (self *FileBasedRingBuffer) Enqueue(item []byte) {
	self.mu.Lock()
	defer self.mu.Unlock()

	if self.closed {
		return
	}

	binary.LittleEndian.PutUint64(self.write_buf, uint64(len(item)))
	_, err := self.fd.WriteAt(self.write_buf, int64(self.header.WritePointer))
	if err != nil {
		self.Reset()
		return
	}
	n, err := self.fd.WriteAt(item, int64(self.header.WritePointer+8))
	if err != nil {
		self.Reset()
		return
	}

	self.header.WritePointer += 8 + int64(n)
	self.header.AvailableBytes += int64(n)

	serialized, _ := self.header.MarshalBinary()
	_, err = self.fd.WriteAt(serialized, 0)
	if err != nil {
		self.Reset()
		return
	}

	logger := logging.GetLogger(self.config_obj, &logging.ClientComponent)
	logger.WithFields(logrus.Fields{
		"header":         json.MustMarshalString(self.header),
		"leased_pointer": self.leased_pointer,
	}).Info("File Ring Buffer: Enqueue")

	// We need to block here until there is room in the message
	// queue. If the message queue is full, the mutex will be
	// locked and we wait here until the data is pushed through to
	// the server, and enough room is available. This has the
	// effect of blocking the executor and stopping the query
	// until we return.
	for self.header.WritePointer > self.header.MaxSize && !self.closed {
		self.c.Wait()
	}
}

func (self *FileBasedRingBuffer) TotalSize() uint64 {
	self.mu.Lock()
	defer self.mu.Unlock()

	return uint64(self.header.AvailableBytes + self.header.LeasedBytes)
}

func (self *FileBasedRingBuffer) AvailableBytes() uint64 {
	self.mu.Lock()
	defer self.mu.Unlock()

	return uint64(self.header.AvailableBytes)
}

// Call Lease() repeatadly and compress each result until we get
// closer to the required size.
func LeaseAndCompress(self IRingBuffer, size uint64) [][]byte {
	result := [][]byte{}
	total_len := uint64(0)
	step := size / 4

	for total_len < size {
		next_message_list := self.Lease(step)

		// No more messages.
		if len(next_message_list) == 0 {
			break
		}

		compressed_message_list, err := utils.Compress(next_message_list)
		if err != nil || len(compressed_message_list) == 0 {
			// Something terrible happened! The file is
			// corrupted and it is better to start again.
			self.Reset()
			break
		}
		result = append(result, compressed_message_list)
		total_len += uint64(len(compressed_message_list))
	}

	return result
}

// Determine if the item is blacklisted. Items are blacklisted when
// their corresponding flow is cancelled.
func (self *FileBasedRingBuffer) IsItemBlackListed(item []byte) bool {
	message_list := crypto_proto.MessageList{}
	err := proto.Unmarshal(item, &message_list)
	if err != nil || len(message_list.Job) == 0 {
		return false
	}

	message := message_list.Job[0]

	// Always allow log messages through - even after a flow has
	// been cancelled. This allows us to register the cancellation
	// message in the flow logs.
	if message.LogMessage != nil {
		return false
	}

	if executor.Canceller != nil {
		return executor.Canceller.IsCancelled(message.SessionId)
	}
	return false
}

func (self *FileBasedRingBuffer) Lease(size uint64) []byte {
	self.mu.Lock()
	defer self.mu.Unlock()

	result := []byte{}

	for self.header.WritePointer > self.leased_pointer {
		n, err := self.fd.ReadAt(self.read_buf, self.leased_pointer)
		if err == nil && n == len(self.read_buf) {
			length := int64(binary.LittleEndian.Uint64(self.read_buf))

			// File might be corrupt - just reset the entire file.
			if length > constants.MAX_MEMORY*2 || length <= 0 {
				self.log_ctx.Error("Possible corruption detected - item length is too large.")
				self._Truncate()
				return nil
			}
			item := make([]byte, length)
			n, err := self.fd.ReadAt(item, self.leased_pointer+8)
			if err != nil || int64(n) != length {
				self.log_ctx.Errorf(
					"Possible corruption detected - expected item of length %v received %v.",
					length, n)
				self._Truncate()
				return nil
			}
			if !self.IsItemBlackListed(item) {
				result = append(result, item...)
			}

			self.leased_pointer += 8 + int64(n)
			self.header.LeasedBytes += int64(n)
			self.header.AvailableBytes -= int64(n)

			if uint64(len(result)) > size {
				break
			}

		} else {
			self.log_ctx.Error("Possible corruption detected: file too short.")
			self._Truncate()
		}
	}

	return result
}

// _Truncate returns the file to a virgin state. Assumes
// FileBasedRingBuffer is already under lock.
func (self *FileBasedRingBuffer) _Truncate() {
	_ = self.fd.Truncate(0)
	self.header.ReadPointer = FirstRecordOffset
	self.header.WritePointer = FirstRecordOffset
	self.header.AvailableBytes = 0
	self.header.LeasedBytes = 0

	self.leased_pointer = FirstRecordOffset
	serialized, _ := self.header.MarshalBinary()
	_, _ = self.fd.WriteAt(serialized, 0)

	// Unblock any blocked writers to let them know there is now room
	// in the file.
	self.c.Broadcast()
}

func (self *FileBasedRingBuffer) Reset() {
	self.mu.Lock()
	defer self.mu.Unlock()

	self._Truncate()
}

func (self *FileBasedRingBuffer) Close() {
	self.mu.Lock()
	defer self.mu.Unlock()

	self.closed = true
	self.fd.Close()
	os.Remove(self.fd.Name())

	// Unblock any blocked writers to let them know this file is now
	// closed.
	self.c.Broadcast()
}

func (self *FileBasedRingBuffer) Commit() {
	self.mu.Lock()
	defer self.mu.Unlock()

	logger := logging.GetLogger(self.config_obj, &logging.ClientComponent)

	// We read up to the write pointer, we may truncate the file now.
	if self.leased_pointer == self.header.WritePointer {
		self._Truncate()
		return
	}

	self.header.ReadPointer = self.leased_pointer
	self.header.LeasedBytes = 0

	serialized, _ := self.header.MarshalBinary()
	_, _ = self.fd.WriteAt(serialized, 0)

	logger.WithFields(logrus.Fields{
		"header": json.MustMarshalString(self.header),
	}).Info("File Ring Buffer: Commit")
}

func NewFileBasedRingBuffer(
	ctx context.Context,
	config_obj *config_proto.Config,
	log_ctx *logging.LogContext) (*FileBasedRingBuffer, error) {

	if config_obj.Client == nil || config_obj.Client.LocalBuffer == nil {
		return nil, errors.New("Local buffer not configured")
	}

	filename := getLocalBufferName(config_obj)
	if filename == "" {
		return nil, errors.New("Unsupport platform")
	}

	fd, err := os.OpenFile(filename, os.O_RDWR|os.O_CREATE, 0700)
	if err != nil {
		return nil, err
	}

	header := &Header{
		// Pad the header a bit to allow for extensions.
		WritePointer:   FirstRecordOffset,
		AvailableBytes: 0,
		LeasedBytes:    0,
		ReadPointer:    FirstRecordOffset,
		MaxSize: int64(config_obj.Client.LocalBuffer.DiskSize) +
			FirstRecordOffset,
	}
	data := make([]byte, FirstRecordOffset)
	n, err := fd.ReadAt(data, 0)
	if n > 0 && n < FirstRecordOffset && err == io.EOF {
		log_ctx.Error("Possible corruption detected: file too short.")
		err = fd.Truncate(0)
		if err != nil {
			return nil, err
		}
	}

	if n > 0 && (err == nil || err == io.EOF) {
		err := header.UnmarshalBinary(data[:n])
		// The header is not valid, truncate the file and
		// start again.
		if err != nil {
			log_ctx.Errorf("Possible corruption detected: %v.", err)
			err = fd.Truncate(0)
			if err != nil {
				return nil, err
			}
		}
	}

	// If we opened a file which is not yet fully committed adjust
	// the available bytes again so we can replay the lost
	// messages.
	if header.LeasedBytes != 0 {
		header.AvailableBytes += header.LeasedBytes
		header.LeasedBytes = 0
	}

	result := &FileBasedRingBuffer{
		config_obj:     config_obj,
		fd:             fd,
		header:         header,
		read_buf:       make([]byte, 8),
		write_buf:      make([]byte, 8),
		leased_pointer: header.ReadPointer,
		log_ctx:        log_ctx,
	}

	result.c = sync.NewCond(&result.mu)

	log_ctx.WithFields(logrus.Fields{
		"filename": filename,
		"max_size": result.header.MaxSize,
	}).Info("Ring Buffer: Creation")

	return result, nil
}

type RingBuffer struct {
	config_obj *config_proto.Config

	// We serialize messages into the messages queue as they
	// arrive.
	mu       sync.Mutex
	messages [][]byte
	closed   bool

	// The index in the messages array where messages before it
	// are leased.
	leased_idx uint64

	// Total length in bytes that is currently leased (this will
	// be several messages since only whole messages are ever
	// leased).
	leased_length uint64

	// Protects total_length
	c            *sync.Cond
	total_length uint64

	// The maximum size of the ring buffer
	Size uint64
}

func (self *RingBuffer) Reset() {
	self.mu.Lock()
	defer self.mu.Unlock()

	self.total_length = 0
	self.messages = nil
	self.c.Broadcast()
}

func (self *RingBuffer) Enqueue(item []byte) {
	self.c.L.Lock()
	defer self.c.L.Unlock()

	if self.closed {
		return
	}

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
	for self.total_length > self.Size && !self.closed {
		self.c.Wait()
	}
}

func (self *RingBuffer) TotalSize() uint64 {
	self.mu.Lock()
	defer self.mu.Unlock()

	return self.total_length
}

func (self *RingBuffer) AvailableBytes() uint64 {
	self.mu.Lock()
	defer self.mu.Unlock()

	return self.total_length - self.leased_length
}

// Determine if the item is blacklisted. Items are blacklisted when
// their corresponding flow is cancelled.
func (self *RingBuffer) IsItemBlackListed(item []byte) bool {
	message_list := crypto_proto.MessageList{}
	err := proto.Unmarshal(item, &message_list)
	if err != nil || len(message_list.Job) == 0 {
		return false
	}

	message := message_list.Job[0]

	// Always allow log messages through - even after a flow has
	// been cancelled. This allows us to register the cancellation
	// message in the flow logs.
	if message.LogMessage != nil {
		return false
	}

	if executor.Canceller != nil {
		return executor.Canceller.IsCancelled(message.SessionId)
	}
	return false
}

// Leases a group of messages for transmission. Will not advance the
// read pointer until we know those have been successfully delivered
// via Commit(). This allows us to crash during transmission and we
// will just re-send the messages when we restart.
// NOTE: This is not used right now - the buffer is reset on startup.
func (self *RingBuffer) Lease(size uint64) []byte {
	self.mu.Lock()
	defer self.mu.Unlock()

	// No more to lease.
	if self.leased_idx >= uint64(len(self.messages)) {
		return nil
	}

	leased := make([]byte, 0)

	for _, item := range self.messages[self.leased_idx:] {
		if !self.IsItemBlackListed(item) {
			leased = append(leased, item...)
		}
		self.leased_length += uint64(len(item))
		self.leased_idx += 1
		if uint64(len(leased)) > size {
			break
		}
	}

	logger := logging.GetLogger(self.config_obj, &logging.ClientComponent)
	logger.WithFields(logrus.Fields{
		"total_length":  len(leased),
		"leased_length": self.leased_length,
	}).Info("Ring Buffer: Leased")

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

func (self *RingBuffer) Close() {
	self.mu.Lock()
	defer self.mu.Unlock()

	self.closed = true
	self.total_length = 0
	self.messages = nil
	self.c.Broadcast()
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

	if uint64(len(self.messages)) >= self.leased_idx {
		self.messages = self.messages[self.leased_idx:]
	}

	self.total_length -= self.leased_length
	self.leased_length = 0
	self.leased_idx = 0

	logger.WithFields(logrus.Fields{
		"total_length": self.total_length,
	}).Info("Ring Buffer: Truncate")

	self.c.Broadcast()
}

func NewRingBuffer(config_obj *config_proto.Config, size uint64) *RingBuffer {
	result := &RingBuffer{
		messages:   make([][]byte, 0),
		Size:       size,
		config_obj: config_obj,
	}
	result.c = sync.NewCond(&result.mu)

	return result
}

func getLocalBufferName(config_obj *config_proto.Config) string {
	switch runtime.GOOS {
	case "windows":
		return os.ExpandEnv(config_obj.Client.LocalBuffer.FilenameWindows)
	case "linux":
		return os.ExpandEnv(config_obj.Client.LocalBuffer.FilenameLinux)
	case "darwin":
		return os.ExpandEnv(config_obj.Client.LocalBuffer.FilenameDarwin)
	default:
		return ""
	}
}

func NewLocalBuffer(
	ctx context.Context,
	config_obj *config_proto.Config) IRingBuffer {
	if config_obj.Client.LocalBuffer.DiskSize > 0 &&
		getLocalBufferName(config_obj) != "" {

		logger := logging.GetLogger(config_obj, &logging.ClientComponent)
		rb, err := NewFileBasedRingBuffer(ctx, config_obj, logger)
		if err == nil {
			return rb
		}
		logger.Error("Unable to create a file based ring buffer - using in memory only.")
	}
	return NewRingBuffer(config_obj, config_obj.Client.LocalBuffer.MemorySize)
}
