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

func (self *TimelineWriter) WriteBuffer(
	timestamp time.Time, serialized []byte) error {

	if len(serialized) == 0 {
		return nil
	}

	offset, err := self.fd.Size()
	if err != nil {
		return err
	}

	out := &bytes.Buffer{}
	offsets := &bytes.Buffer{}

	// Write line delimited JSON
	out.Write(serialized)

	// Only add a single lf if needed.
	if serialized[len(serialized)-1] != '\n' {
		out.Write([]byte{'\n'})
	}

	idx_record := &IndexRecord{
		Timestamp: timestamp.UnixNano(),
		Offset:    offset,
	}

	err = binary.Write(offsets, binary.LittleEndian, idx_record)
	if err != nil {
		return err
	}

	// Include the line feed in the count.
	offset += int64(len(serialized) + 1)

	_, err = self.fd.Write(out.Bytes())
	if err != nil {
		return err
	}
	_, err = self.index_fd.Write(offsets.Bytes())
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
