// Manage reading and writing result sets.

// Velociraptor is essentially a VQL engine - all operations are
// simply queries and all queries return a result set. Result sets are
// essentially tables - containing columns specified by the query
// itself and rows.

// This module manages storing the result sets in the data
// store. Result sets are written using a ResultSetWriter - which can
// create a new result set or append to an existing result set.

// Rows in the result set are written in JSONL to a file, and their
// index is maintained. A ResultSetReader can be used to retrieve rows
// efficiently.

// What does the index look like? The index consists of a series of
// uint64 integers, one per row in the main file. The lower 40 bits
// represent the offset into the JSON file of bulk data. The upper 24
// bits are a count of rows from the start of the blob.

// For example, if a blob containing 20 rows is appended to the main
// file, the index will consist of 20 uint64, each low bits are the
// offset to the start of the blob, and each will have an incrementing
// upper 24 bits.

package simple

import (
	"bufio"
	"bytes"
	"context"
	"encoding/binary"
	"errors"
	"io"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/Velocidex/json"
	"github.com/Velocidex/ordereddict"
	"www.velocidex.com/golang/velociraptor/file_store/api"
	vjson "www.velocidex.com/golang/velociraptor/json"
	"www.velocidex.com/golang/velociraptor/result_sets"
	"www.velocidex.com/golang/velociraptor/utils"
)

var (
	retransmissionError = errors.New("RetransmissionError")
)

const (
	offset_mask = 1<<40 - 1
)

type ResultSetWriterImpl struct {
	mu       sync.Mutex
	rows     [][]byte
	opts     *json.EncOpts
	fd       api.FileWriter
	index_fd api.FileWriter

	file_store_factory api.FileStore
	log_path           api.FSPathSpec

	sync bool
}

// This tells us that we expect to write the next row at this offset.
// We need to ensure the file is actually as we expect it to be.
func (self *ResultSetWriterImpl) SetStartRow(start_row int64) error {
	self.mu.Lock()
	defer self.mu.Unlock()

	// Calculate the number of rows currently in the file.
	idx_size, err := self.index_fd.Size()
	if err != nil {
		return err
	}

	// The numebr of rows in the underlying file.
	number_of_rows := idx_size / 8 // 8 Bytes per row in the index.

	// Corrent for any rows we have in memory waiting to be flushed.
	number_of_rows += int64(len(self.rows))

	// This is a retransmission
	if number_of_rows > start_row {
		return retransmissionError
	}

	return nil
}

func (self *ResultSetWriterImpl) SetSync() {
	self.mu.Lock()
	defer self.mu.Unlock()

	self.sync = true
}

// WriteJSONL writes an entire JSONL blob to the end of the result
// set. This is supposed to be very fast so we dont have to parse the
// JSON (Typically the client sends us the complete JSON blob).  Since
// we do not not know exactly where in the JSON blob each row starts
// we update the index to refer to the begining of the row and the
// number of rows from there.

// The reader will find the correct row by loading the JSONL file at
// the indicated offset then reading lines off it until they reach the
// desired row index.
func (self *ResultSetWriterImpl) WriteJSONL(serialized []byte, total_rows uint64) {
	if total_rows == 0 {
		total_rows = countLines(serialized)
	}

	// Sync the index with the current buffers.
	self.Flush()

	// Write an index that spans the serialized range.
	offset, err := self.fd.Size()
	if err != nil {
		return
	}

	// All the index slots will point to the start of the blob
	offsets := new(bytes.Buffer)
	for i := uint64(0); i < total_rows; i++ {
		value := uint64(offset) | (i << 40)
		err = binary.Write(offsets, binary.LittleEndian, value)
		if err != nil {
			return
		}
	}

	_, _ = self.fd.Write(serialized)
	_, _ = self.index_fd.Write(offsets.Bytes())
}

func (self *ResultSetWriterImpl) WriteCompressedJSONL(
	serialized []byte, byte_offset uint64, uncompressed_size int,
	total_rows uint64) {

	// Sync the index with the current buffers.
	self.Flush()

	// Write an index that spans the serialized range.
	offset, err := self.fd.Size()
	if err != nil {
		return
	}

	// All the index slots will point to the start of the blob
	offsets := new(bytes.Buffer)
	for i := uint64(0); i < total_rows; i++ {
		value := uint64(offset) | (i << 40)
		err = binary.Write(offsets, binary.LittleEndian, value)
		if err != nil {
			return
		}
	}

	_, _ = self.fd.WriteCompressed(serialized, byte_offset, uncompressed_size)
	_, _ = self.index_fd.Write(offsets.Bytes())
}

func (self *ResultSetWriterImpl) Write(row *ordereddict.Dict) {
	self.mu.Lock()
	defer self.mu.Unlock()

	// Encode each row ASAP but then store the raw json for combined
	// writes. This allows us to get rid of memory ASAP.
	serialized, err := vjson.MarshalWithOptions(row, self.opts)
	if err != nil {
		return
	}

	self.rows = append(self.rows, serialized)
	if len(self.rows) > 10000 {
		self._Flush()
	}
}

// Do not actually write the data until Close() or Flush() are called,
// or until 10k rows are queued in memory.
func (self *ResultSetWriterImpl) Flush() {
	self.mu.Lock()
	defer self.mu.Unlock()

	if len(self.rows) > 0 {
		self._Flush()
	}
}

func (self *ResultSetWriterImpl) _Flush() {
	offset, err := self.fd.Size()
	if err != nil {
		return
	}

	out := &bytes.Buffer{}
	offsets := new(bytes.Buffer)
	for _, row := range self.rows {

		// Write line delimited JSON
		out.Write(row)
		out.Write([]byte{'\n'})
		err = binary.Write(offsets, binary.LittleEndian, offset)
		if err != nil {
			return
		}

		// Include the line feed in the count.
		offset += int64(len(row) + 1)
	}

	_, _ = self.fd.Write(out.Bytes())
	_, _ = self.index_fd.Write(offsets.Bytes())

	// Reset the slice but keep the capacity.
	self.rows = self.rows[:0]
}

func (self *ResultSetWriterImpl) Close() {
	self.Flush()
	self.fd.Close()
	self.index_fd.Close()

	if self.sync {
		self.fd.Flush()
		self.index_fd.Flush()
	}
}

type ResultSetFactory struct{}

func (self ResultSetFactory) NewResultSetWriter(
	file_store_factory api.FileStore,
	log_path api.FSPathSpec,
	opts *json.EncOpts,
	completion func(),
	truncate result_sets.WriteMode) (result_sets.ResultSetWriter, error) {

	result := &ResultSetWriterImpl{
		opts:               opts,
		file_store_factory: file_store_factory,
		log_path:           log_path,
	}

	// If no path is provided, we are just a log sink
	if utils.IsNil(log_path) {
		return &NullResultSetWriter{}, nil
	}

	// Call the completion when both files are done.
	completer := utils.NewCompleter(completion)

	fd, err := file_store_factory.WriteFileWithCompletion(
		log_path, completer.GetCompletionFunc())
	if err != nil {
		return nil, err
	}

	idx_fd, err := file_store_factory.WriteFileWithCompletion(
		log_path.SetType(api.PATH_TYPE_FILESTORE_JSON_INDEX),
		completer.GetCompletionFunc())
	if err != nil {
		fd.Close()
		return nil, err
	}

	if truncate {
		err = fd.Truncate()
		if err != nil {
			fd.Close()
			idx_fd.Close()
			return nil, err
		}

		err = idx_fd.Truncate()
		if err != nil {
			fd.Close()
			idx_fd.Close()
			return nil, err
		}

	}

	result.fd = fd
	result.index_fd = idx_fd

	return result, nil
}

// A ResultSetReader can produce rows from a result set.
type ResultSetReaderImpl struct {
	total_rows int64
	mtime      time.Time

	fd       api.FileReader
	idx_fd   api.FileReader
	log_path api.FSPathSpec

	stacker api.FSPathSpec
}

func (self *ResultSetReaderImpl) SetStacker(stacker api.FSPathSpec) {
	self.stacker = stacker
}

func (self *ResultSetReaderImpl) Stacker() api.FSPathSpec {
	return self.stacker
}

func (self *ResultSetReaderImpl) TotalRows() int64 {
	return self.total_rows
}

func (self *ResultSetReaderImpl) MTime() time.Time {
	return self.mtime
}

// Seeks the fd to the starting location. If successful then fd is
// ready to be read from row at a time.
func (self *ResultSetReaderImpl) SeekToRow(start int64) error {
	// Nothing to do.
	if start == 0 {
		return nil
	}

	if self.idx_fd == nil {
		// There is no index file, we fallback to reading slowly
		reader := bufio.NewReader(self.fd)
		for i := int64(0); i < start; i++ {
			_, err := reader.ReadBytes('\n')
			if err != nil {
				return err
			}
		}
		return nil
	}

	// Get the index entry for this row
	_, err := self.idx_fd.Seek(8*start, io.SeekStart)
	if err != nil {
		return err
	}

	value := int64(0)
	err = binary.Read(self.idx_fd, binary.LittleEndian, &value)
	if err != nil {
		return err
	}

	// The value contains the file offset and the row count.
	offset := value & offset_mask
	row_count := value >> 40

	// Seek to the start of the row in the index.
	_, err = self.fd.Seek(offset, io.SeekStart)
	if err != nil {
		return err
	}

	// We are at the correct spot
	if row_count == 0 {
		return nil
	}

	// Consume rows from the start of the blob to reach our
	// desired row count.
	reader := bufio.NewReader(self.fd)
	for i := int64(0); i < row_count; i++ {
		line, err := reader.ReadBytes('\n')
		if err != nil {
			return err
		}
		offset += int64(len(line))
	}

	// Got there! now seek back to the correct spot
	_, err = self.fd.Seek(offset, io.SeekStart)
	return err
}

// Start generating rows from the result set.
func (self *ResultSetReaderImpl) Rows(ctx context.Context) <-chan *ordereddict.Dict {
	output := make(chan *ordereddict.Dict)

	go func() {
		defer close(output)

		reader := bufio.NewReader(self.fd)
		for {
			row_data, err := reader.ReadBytes('\n')
			if err != nil {
				return
			}

			// We have reached the end.
			if len(row_data) == 0 {
				return
			}

			if len(row_data) < 2 {
				continue
			}

			// This is a pointer to the real record.
			if row_data[0] == '@' {
				ptr_offset, err := strconv.ParseInt(
					strings.Trim(string(row_data), "@\n"), 0, 64)
				if err != nil {
					continue
				}

				current_offset, err := self.fd.Seek(0, os.SEEK_CUR)
				if err != nil {
					return
				}

				_, err = self.fd.Seek(ptr_offset, os.SEEK_SET)
				if err != nil {
					return
				}

				// Make a new private buffer so as not to disturb the
				// original buffer.
				reader := bufio.NewReader(self.fd)
				row_data, err = reader.ReadBytes('\n')
				if err != nil {
					return
				}

				// Seek back to the correct position
				_, err = self.fd.Seek(current_offset, os.SEEK_SET)
				if err != nil {
					return
				}

				if len(row_data) < 2 {
					continue
				}
				replacement := &replacement_record{}
				err = json.Unmarshal(row_data[1:], replacement)
				if err != nil {
					continue
				}

				row_data = replacement.Data
			}

			item := ordereddict.NewDict()

			// We failed to unmarshal one line of
			// JSON - it may be corrupted, go to
			// the next one.
			err = item.UnmarshalJSON(row_data)
			if err != nil {
				continue
			}

			select {
			case <-ctx.Done():
				return
			case output <- item:
			}
		}
	}()
	return output
}

// Start generating rows from the result set.
func (self *ResultSetReaderImpl) JSON(ctx context.Context) (<-chan []byte, error) {
	output := make(chan []byte)

	go func() {
		defer close(output)

		reader := bufio.NewReader(self.fd)
		for {
			row_data, err := reader.ReadBytes('\n')
			if err != nil {
				return
			}

			// We have reached the end.
			if len(row_data) == 0 {
				return
			}

			select {
			case <-ctx.Done():
				return
			case output <- row_data:
			}
		}
	}()

	return output, nil
}

// Only used in tests - not safe for general use.
func GetAllResults(self result_sets.ResultSetReader) []*ordereddict.Dict {
	result := []*ordereddict.Dict{}
	for row := range self.Rows(context.Background()) {
		result = append(result, row)
	}
	return result
}

func (self *ResultSetReaderImpl) Close() {
	self.fd.Close()
	if self.idx_fd != nil {
		self.idx_fd.Close()
	}
}

type NullReader struct {
	*bytes.Reader
	pathSpec_ api.FSPathSpec
}

func (self NullReader) PathSpec() api.FSPathSpec {
	return self.pathSpec_
}

func (self NullReader) Close() error {
	return nil
}

func (self NullReader) Stat() (api.FileInfo, error) {
	return nil, errors.New("Not found")
}

func (self ResultSetFactory) DeleteResultSet(
	file_store_factory api.FileStore,
	path api.FSPathSpec) (err error) {

	// A result set consists of:
	// 1. The main jsonl file
	// 2. An index jsonl file
	// 3. optionally a chunk file for compressed result sets
	// 4. A directory hierarchy of transformed cache files.

	// Try to delete these but dont worry if they are missing
	_ = file_store_factory.Delete(path.
		SetType(api.PATH_TYPE_FILESTORE_JSON_INDEX))

	_ = file_store_factory.Delete(path.
		SetType(api.PATH_TYPE_FILESTORE_CHUNK_INDEX))

	err = file_store_factory.Delete(path)
	if err != nil {
		return err
	}

	deleter := func(urn api.FSPathSpec, info os.FileInfo) error {
		return file_store_factory.Delete(urn)
	}
	_ = api.Walk(file_store_factory,
		path.AddChild("sorted"), deleter)

	_ = api.Walk(file_store_factory,
		path.AddChild("filtered"), deleter)

	return err
}

func (self ResultSetFactory) NewResultSetReader(
	file_store_factory api.FileStore,
	log_path api.FSPathSpec) (result_sets.ResultSetReader, error) {

	if file_store_factory == nil {
		return nil, errors.New("No filestore")
	}

	fd, err := file_store_factory.ReadFile(log_path)
	if errors.Is(err, io.EOF) || errors.Is(err, os.ErrNotExist) {
		fd = &NullReader{
			Reader:    bytes.NewReader([]byte{}),
			pathSpec_: log_path,
		}
	} else if err != nil {
		return nil, err
	}
	// Keep the open file until the reader is closed.

	// -1 indicates we dont know how many rows there are
	total_rows := int64(-1)
	var mtime time.Time
	idx_fd, err := file_store_factory.ReadFile(log_path.
		SetType(api.PATH_TYPE_FILESTORE_JSON_INDEX))
	if err == nil {
		stat, err := idx_fd.Stat()
		if err == nil {
			total_rows = stat.Size() / 8
			mtime = stat.ModTime()
		}
	}

	if os.IsNotExist(err) {
		idx_fd = &NullReader{
			Reader:    bytes.NewReader([]byte{}),
			pathSpec_: log_path,
		}
	}

	return &ResultSetReaderImpl{
		total_rows: total_rows,
		mtime:      mtime,
		fd:         fd,
		idx_fd:     idx_fd,
		log_path:   log_path,
	}, nil
}

func countLines(serialized []byte) uint64 {
	var result uint64
	for _, i := range serialized {
		if i == '\n' {
			result++
		}
	}
	return result
}

func init() {
	result_sets.RegisterResultSetFactory(ResultSetFactory{})
}
