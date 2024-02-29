// A ring buffer to queue messages

// Similar to the client ring buffer but this one has no limit because
// we never want to block writers.

package journal

import (
	"encoding/binary"
	"errors"
	"io"
	"os"
	"sync"

	"google.golang.org/protobuf/proto"
	api_proto "www.velocidex.com/golang/velociraptor/api/proto"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	"www.velocidex.com/golang/velociraptor/constants"
	logging "www.velocidex.com/golang/velociraptor/logging"
)

// The below is similar to http_comms.FileBasedRingBuffer except:
// * Size of the file is not limited.
// * Leasing a single message at once.
// * Messages are of type api_proto.PushEventRequest

const (
	FileMagic         = "VRB\x5f"
	FirstRecordOffset = 50
)

var (
	ErrorsCorrupted = errors.New("File is corrupted")
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

type BufferFile struct {
	config_obj *config_proto.Config

	mu sync.Mutex

	fd     *os.File
	Header *Header

	read_buf  []byte
	write_buf []byte

	log_ctx *logging.LogContext
}

func (self *BufferFile) GetHeader() Header {
	self.mu.Lock()
	defer self.mu.Unlock()
	return *self.Header
}

// Enqueue the item into the ring buffer and append to the end.
func (self *BufferFile) Enqueue(item *api_proto.PushEventRequest) error {
	serialized, err := proto.Marshal(item)
	if err != nil {
		return err
	}

	self.mu.Lock()
	defer self.mu.Unlock()

	// Write the new message to the end of the file at the WritePointer
	binary.LittleEndian.PutUint64(self.write_buf, uint64(len(serialized)))
	_, err = self.fd.WriteAt(self.write_buf, int64(self.Header.WritePointer))
	if err != nil {
		// File is corrupt now, reset it.
		self._Truncate()
		return err
	}

	n, err := self.fd.WriteAt(serialized, int64(self.Header.WritePointer+8))
	if err != nil {
		self._Truncate()
		return err
	}

	self.Header.WritePointer += 8 + int64(n)

	// Update the header
	serialized, err = self.Header.MarshalBinary()
	if err != nil {
		return err
	}
	_, err = self.fd.WriteAt(serialized, 0)
	if err != nil {
		self._Truncate()
		return err
	}

	return nil
}

// Returns some messages message from the file.
func (self *BufferFile) Lease() (*api_proto.PushEventRequest, error) {
	self.mu.Lock()
	defer self.mu.Unlock()

	result := &api_proto.PushEventRequest{}

	// The file is empty.
	if self.Header.WritePointer <= self.Header.ReadPointer {
		return nil, io.EOF
	}

	// Read the next chunk (length+value) from the current leased pointer.
	n, err := self.fd.ReadAt(self.read_buf, self.Header.ReadPointer)
	if err != nil || n != len(self.read_buf) {
		self.log_ctx.Error("Possible corruption detected: file too short.")
		self._Truncate()
		return nil, ErrorsCorrupted
	}

	length := int64(binary.LittleEndian.Uint64(self.read_buf))
	// File might be corrupt - just reset the
	// entire file.
	if length > constants.MAX_MEMORY*2 || length <= 0 {
		self.log_ctx.Error("Possible corruption detected - item length is too large.")
		self._Truncate()
		return nil, ErrorsCorrupted
	}

	// Unmarshal one item at a time.
	serialized := make([]byte, length)
	n, _ = self.fd.ReadAt(serialized, self.Header.ReadPointer+8)
	if int64(n) != length {
		self.log_ctx.Errorf(
			"Possible corruption detected - expected item of length %v received %v.",
			length, n)
		self._Truncate()
		return nil, ErrorsCorrupted
	}

	err = proto.Unmarshal(serialized, result)
	if err != nil {
		self.log_ctx.Errorf(
			"Possible corruption detected - unable to decode item.")
		self._Truncate()
		return nil, ErrorsCorrupted
	}

	// Advance the read pointer
	self.Header.ReadPointer += 8 + int64(n)

	// We read up to the write pointer, we may truncate the file
	// now.
	if self.Header.ReadPointer == self.Header.WritePointer {
		self._Truncate()
	}

	return result, nil
}

// _Truncate returns the file to a virgin state. Assumes
// FileBasedRingBuffer is already under lock.
func (self *BufferFile) _Truncate() {
	_ = self.fd.Truncate(0)
	self.Header.ReadPointer = FirstRecordOffset
	self.Header.WritePointer = FirstRecordOffset
	serialized, _ := self.Header.MarshalBinary()
	_, _ = self.fd.WriteAt(serialized, 0)
}

func (self *BufferFile) Reset() {
	self.mu.Lock()
	defer self.mu.Unlock()

	self._Truncate()
}

// Closes the underlying file and shut down the readers.
func (self *BufferFile) Close() {
	self.fd.Close()
	os.Remove(self.fd.Name())
}

func NewBufferFile(
	config_obj *config_proto.Config, fd *os.File) (*BufferFile, error) {

	log_ctx := logging.GetLogger(config_obj, &logging.FrontendComponent)

	header := &Header{
		// Pad the header a bit to allow for extensions.
		WritePointer: FirstRecordOffset,
		ReadPointer:  FirstRecordOffset,
	}
	data := make([]byte, FirstRecordOffset)
	n, err := fd.ReadAt(data, 0)
	if n > 0 && n < FirstRecordOffset && errors.Is(err, io.EOF) {
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

	result := &BufferFile{
		config_obj: config_obj,
		fd:         fd,
		Header:     header,
		read_buf:   make([]byte, 8),
		write_buf:  make([]byte, 8),
		log_ctx:    log_ctx,
	}

	return result, nil
}
