package timelines

/*
  Implements a ITimelineWriter interface to write time series data to
  the filestore.

  Assumes data is written in time increasing order.
*/

import (
	"bytes"
	"encoding/binary"
	"sync"
	"time"

	"github.com/Velocidex/ordereddict"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	"www.velocidex.com/golang/velociraptor/file_store"
	"www.velocidex.com/golang/velociraptor/file_store/api"
	"www.velocidex.com/golang/velociraptor/json"
	vjson "www.velocidex.com/golang/velociraptor/json"
	"www.velocidex.com/golang/velociraptor/paths"
	"www.velocidex.com/golang/velociraptor/result_sets"
	timelines_proto "www.velocidex.com/golang/velociraptor/timelines/proto"
	"www.velocidex.com/golang/velociraptor/utils"
)

const (
	IndexRecordSize = 24
)

type IndexRecord struct {
	//  NanoSeconds
	Timestamp int64

	// Offset to the row data
	Offset int64

	// Annotation Offset
	Annotation int64
}

type TimelineWriter struct {
	mu                    sync.Mutex
	wg                    sync.WaitGroup
	first_time, last_time time.Time
	opts                  *json.EncOpts
	fd                    api.FileWriter
	index_fd              api.FileWriter
}

func (self *TimelineWriter) Stats() *timelines_proto.Timeline {
	self.mu.Lock()
	defer self.mu.Unlock()

	return &timelines_proto.Timeline{
		StartTime: self.first_time.Unix(),
		EndTime:   self.last_time.Unix(),
	}
}

func (self *TimelineWriter) Write(
	timestamp time.Time, row *ordereddict.Dict) error {
	self.mu.Lock()
	if self.first_time.IsZero() {
		self.first_time = timestamp
	}
	self.last_time = timestamp
	self.mu.Unlock()

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
	_ = self.fd.Truncate()
	_ = self.index_fd.Truncate()
}

func (self *TimelineWriter) Close() {
	self.fd.Close()
	self.index_fd.Close()
	self.wg.Wait()
}

func NewTimelineWriter(
	config_obj *config_proto.Config,
	path_manager paths.TimelinePathManagerInterface,
	completion func(),
	truncate result_sets.WriteMode) (*TimelineWriter, error) {

	result := &TimelineWriter{}

	// Call the completer when both index and file are done.
	completer := utils.NewCompleter(completion)

	file_store_factory := file_store.GetFileStore(config_obj)

	fd, err := file_store_factory.WriteFileWithCompletion(
		path_manager.Path(), completer.GetCompletionFunc())
	if err != nil {
		return nil, err
	}

	index_fd, err := file_store_factory.WriteFileWithCompletion(
		path_manager.Index(), completer.GetCompletionFunc())
	if err != nil {
		fd.Close()
		return nil, err
	}

	if truncate {
		_ = fd.Truncate()
		_ = index_fd.Truncate()
	}

	result.fd = fd
	result.index_fd = index_fd

	return result, nil

}
