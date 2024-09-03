// A ring buffer to queue messages

// Similar to the client ring buffer but this one has no limit because
// we never want to block writers.

package directory

import (
	"encoding/binary"
	"errors"
	"os"
	"sync"

	"github.com/Velocidex/ordereddict"
	"github.com/sirupsen/logrus"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	"www.velocidex.com/golang/velociraptor/constants"
	"www.velocidex.com/golang/velociraptor/json"
	logging "www.velocidex.com/golang/velociraptor/logging"
	"www.velocidex.com/golang/velociraptor/utils/tempfile"
	utils_tempfile "www.velocidex.com/golang/velociraptor/utils/tempfile"
)

// The below is similar to http_comms.FileBasedRingBuffer except:
// * Size of the file is not limited.
// * Leasing a full number of messages at once (rather than combined size).

const (
	FileMagic         = "VRB\x5e"
	FirstRecordOffset = 50
)

type Header struct {
	ReadPointer  int64 // Leasing will start at this file offset.
	WritePointer int64 // Enqueue will write at this file position.
}

func (self *Header) MarshalBinary() ([]byte, error) {
	data := make([]byte, FirstRecordOffset)
	copy(data, FileMagic)

	binary.LittleEndian.PutUint64(data[4:12], uint64(self.ReadPointer))
	binary.LittleEndian.PutUint64(data[12:20], uint64(self.WritePointer))

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

	return nil
}

type FileBasedRingBuffer struct {
	config_obj *config_proto.Config

	mu sync.Mutex

	base_name string
	fd        *os.File
	header    *Header

	read_buf  []byte
	write_buf []byte

	log_ctx *logging.LogContext

	// Keep track of how many messages are leased. When we lease
	// messages this wg is added, then callers can decrement it as
	// needed.
	Wg sync.WaitGroup

	max_size int64
}

// Enqueue the item into the ring buffer and append to the end.
func (self *FileBasedRingBuffer) Enqueue(item interface{}) error {
	serialized, err := json.Marshal(item)
	if err != nil {
		return err
	}

	self.mu.Lock()
	defer self.mu.Unlock()

	fd, err := self.getFd()
	if err != nil {
		return err
	}

	// If the file is too large we truncate it and report that we lost
	// some data.
	if self.max_size > 0 && self.header.WritePointer > self.max_size {
		logger := logging.GetLogger(
			self.config_obj, &logging.FrontendComponent)
		logger.WithFields(logrus.Fields{
			"name":       fd.Name(),
			"bytes_lost": self.header.WritePointer - self.header.ReadPointer,
		}).Error("Buffer file too large")
		self._Truncate()
	}

	// Write the new message to the end of the file at the WritePointer
	binary.LittleEndian.PutUint64(self.write_buf, uint64(len(serialized)))
	_, err = fd.WriteAt(self.write_buf, int64(self.header.WritePointer))
	if err != nil {
		// File is corrupt now, reset it.
		self._Truncate()
		return err
	}

	n, err := fd.WriteAt(serialized, int64(self.header.WritePointer+8))
	if err != nil {
		self._Truncate()
		return err
	}

	self.header.WritePointer += 8 + int64(n)

	// Update the header
	serialized, err = self.header.MarshalBinary()
	if err != nil {
		return err
	}
	_, err = fd.WriteAt(serialized, 0)
	if err != nil {
		self._Truncate()
		return err
	}

	return nil
}

// Returns some messages message from the file.
func (self *FileBasedRingBuffer) Lease(count int) []*ordereddict.Dict {
	self.mu.Lock()
	defer self.mu.Unlock()

	result := make([]*ordereddict.Dict, 0, count)

	// If there is no backing file there are no messages to be leased.
	if self.fd == nil || self.header == nil {
		return nil
	}

	fd, err := self.getFd()
	if err != nil || fd == nil {
		return nil
	}

	// The file contains more data.
	for self.header.WritePointer > self.header.ReadPointer {
		// Read the next chunk (length+value) from the current leased pointer.
		n, err := fd.ReadAt(self.read_buf, self.header.ReadPointer)
		if err != nil || n != len(self.read_buf) {
			self.log_ctx.Error(
				"Possible corruption detected: file too short Writer %v, Reader %v.",
				self.header.WritePointer, self.header.ReadPointer)
			self._Truncate()
			return nil
		}

		length := int64(binary.LittleEndian.Uint64(self.read_buf))
		// File might be corrupt - just reset the
		// entire file.
		if length > constants.MAX_MEMORY*2 || length <= 0 {
			self.log_ctx.Error("Possible corruption detected - item length is too large.")
			self._Truncate()
			return nil
		}

		// Unmarshal one item at a time.
		serialized := make([]byte, length)
		n, _ = fd.ReadAt(serialized, self.header.ReadPointer+8)
		if int64(n) != length {
			self.log_ctx.Errorf(
				"Possible corruption detected - expected item of length %v received %v.",
				length, n)
			self._Truncate()
			return nil
		}

		item := ordereddict.NewDict()
		err = item.UnmarshalJSON(serialized)
		if err == nil {
			result = append(result, item)
		}

		self.header.ReadPointer += 8 + int64(n)

		// We read up to the write pointer, we may truncate the file now.
		if self.header.ReadPointer == self.header.WritePointer {
			self._Truncate()
			self.Wg.Add(len(result))
			return result
		}

		if len(result) >= count {
			break
		}
	}

	self.Wg.Add(len(result))
	return result
}

// _Truncate returns the buffer to a virgin state - it removed the
// backing file so next call to getFd() will reuse it.  Assume that
// FileBasedRingBuffer is already under lock.
func (self *FileBasedRingBuffer) _Truncate() {
	if self.fd != nil {
		self.header = nil
		self.fd.Close()

		filename := self.fd.Name()
		err := os.Remove(filename) // clean up file buffer
		utils_tempfile.RemoveTmpFile(filename, err)

		self.fd = nil
	}
}

func (self *FileBasedRingBuffer) PendingSize() int64 {
	self.mu.Lock()
	defer self.mu.Unlock()

	if self.header == nil {
		return 0
	}

	return self.header.WritePointer - self.header.ReadPointer
}

func (self *FileBasedRingBuffer) Reset() {
	self.mu.Lock()
	defer self.mu.Unlock()

	self._Truncate()
}

// Closes the underlying file and shut down the readers.
func (self *FileBasedRingBuffer) Close() {
	self.mu.Lock()
	defer self.mu.Unlock()

	if self.fd != nil {
		self.fd.Close()
		self.fd = nil
		self.header = nil
	}
}

func (self *FileBasedRingBuffer) getFd() (*os.File, error) {
	if self.fd != nil {
		return self.fd, nil
	}

	// Create a tempfile on demand.
	self.header = &Header{
		WritePointer: FirstRecordOffset,
		ReadPointer:  FirstRecordOffset,
	}

	fd, err := tempfile.TempFile(self.base_name)
	if err != nil {
		return nil, err
	}

	utils_tempfile.AddTmpFile(fd.Name())

	self.fd = fd

	return self.fd, nil
}

func (self *FileBasedRingBuffer) GetBackingFile() string {
	self.mu.Lock()
	defer self.mu.Unlock()

	if self.fd == nil {
		return ""
	}

	return self.fd.Name()
}

func NewFileBasedRingBuffer(
	config_obj *config_proto.Config,
	base_name string) (*FileBasedRingBuffer, error) {

	log_ctx := logging.GetLogger(config_obj, &logging.FrontendComponent)
	header := &Header{
		// Pad the header a bit to allow for extensions.
		WritePointer: FirstRecordOffset,
		ReadPointer:  FirstRecordOffset,
	}

	result := &FileBasedRingBuffer{
		config_obj: config_obj,
		base_name:  base_name,
		header:     header,
		read_buf:   make([]byte, 8),
		write_buf:  make([]byte, 8),
		log_ctx:    log_ctx,
		max_size:   1024 * 1024 * 1024, // 1Gb
	}

	if config_obj.Frontend != nil && config_obj.Frontend.Resources != nil &&
		config_obj.Frontend.Resources.MaxJournalBufferSize > 0 {
		result.max_size = config_obj.Frontend.Resources.MaxJournalBufferSize
	}

	return result, nil
}
