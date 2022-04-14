package timelines

import (
	"bytes"
	"encoding/binary"
	"time"

	"github.com/Velocidex/ordereddict"
	"www.velocidex.com/golang/velociraptor/file_store/api"
	"www.velocidex.com/golang/velociraptor/json"
	vjson "www.velocidex.com/golang/velociraptor/json"
	"www.velocidex.com/golang/velociraptor/paths"
	"www.velocidex.com/golang/velociraptor/result_sets"
)

const (
	IndexRecordSize = 24
)

type IndexRecord struct {
	Timestamp int64

	// Offset to the row data
	Offset int64

	// Annotation Offset
	Annotation int64
}

type TimelineWriter struct {
	last_time time.Time
	opts      *json.EncOpts
	fd        api.FileWriter
	index_fd  api.FileWriter
}

func (self *TimelineWriter) Write(
	timestamp time.Time, row *ordereddict.Dict) error {
	serialized, err := vjson.MarshalWithOptions(row, self.opts)
	if err != nil {
		return err
	}

	return self.WriteBuffer(timestamp, serialized)
}

// Write potentially multiple rows into the file at the same
// timestamp.
func (self *TimelineWriter) WriteBuffer(
	timestamp time.Time, serialized []byte) error {

	if len(serialized) == 0 {
		return nil
	}

	// Only add a single lf if needed. Serialized must end with a
	// single \n.
	if serialized[len(serialized)-1] != '\n' {
		serialized = append(serialized, '\n')
	}

	offset, err := self.fd.Size()
	if err != nil {
		return err
	}

	// A buffer to prepare the index in memory.
	offsets := &bytes.Buffer{}

	// Prepare the index records without parsing the actual JSON.
	for idx, c := range serialized {
		// A LF represents the end of the record.
		if idx == 0 ||
			idx < len(serialized)-1 && c == '\n' {
			idx_record := &IndexRecord{
				Timestamp: timestamp.UnixNano(),
				Offset:    offset + int64(idx),
			}
			err = binary.Write(offsets, binary.LittleEndian, idx_record)
			if err != nil {
				return err
			}
		}
	}

	// Write the index data
	_, err = self.index_fd.Write(offsets.Bytes())
	if err != nil {
		return err
	}

	// Write the bulk data
	_, err = self.fd.Write(serialized)
	return err
}

func (self *TimelineWriter) Truncate() {
	self.fd.Truncate()
	self.index_fd.Truncate()
}

func (self *TimelineWriter) Close() {
	self.fd.Close()
	self.index_fd.Close()
}

func NewTimelineWriter(
	file_store_factory api.FileStore,
	path_manager paths.TimelinePathManagerInterface,
	completion func(),
	truncate result_sets.WriteMode) (*TimelineWriter, error) {

	result := &TimelineWriter{}

	fd, err := file_store_factory.WriteFileWithCompletion(
		path_manager.Path(), completion)
	if err != nil {
		return nil, err
	}

	index_fd, err := file_store_factory.WriteFile(
		path_manager.Index())
	if err != nil {
		fd.Close()
		return nil, err
	}

	if truncate {
		fd.Truncate()
		index_fd.Truncate()
	}

	result.fd = fd
	result.index_fd = index_fd

	return result, nil

}
