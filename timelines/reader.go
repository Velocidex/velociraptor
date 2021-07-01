package timelines

import (
	"bufio"
	"context"
	"encoding/binary"
	"sort"
	"time"

	"github.com/Velocidex/ordereddict"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	"www.velocidex.com/golang/velociraptor/file_store"
	"www.velocidex.com/golang/velociraptor/file_store/api"
	"www.velocidex.com/golang/velociraptor/glob"
)

type TimelineItem struct {
	Row  *ordereddict.Dict
	Time int64
}

type TimelineReader struct {
	current_idx int
	fd          api.FileReader
	index_fd    api.FileReader
	index_stat  glob.FileInfo
}

func (self *TimelineReader) getIndex(i int) *IndexRecord {
	idx_record := &IndexRecord{}

	_, err := self.index_fd.Seek(int64(i)*IndexRecordSize, 0)
	if err != nil {
		return idx_record
	}

	_ = binary.Read(self.index_fd, binary.LittleEndian, idx_record)
	return idx_record
}

func (self *TimelineReader) SeekToTime(timestamp time.Time) {
	timestamp_int := timestamp.UnixNano()
	number_of_points := self.index_stat.Size() / IndexRecordSize

	self.current_idx = sort.Search(int(number_of_points), func(i int) bool {
		// Read the index record at offset i
		idx_record := self.getIndex(i)
		return idx_record.Timestamp >= timestamp_int
	})
	idx_record := self.getIndex(self.current_idx)
	self.fd.Seek(idx_record.Offset, 0)
}

func (self *TimelineReader) Read(ctx context.Context) <-chan TimelineItem {
	output_chan := make(chan TimelineItem)

	go func() {
		defer close(output_chan)

		reader := bufio.NewReader(self.fd)
		for {
			select {
			case <-ctx.Done():
				return

			default:
				row_data, err := reader.ReadBytes('\n')
				if err != nil {
					return
				}

				// We have reached the end.
				if len(row_data) == 0 {
					return
				}

				idx_record := self.getIndex(self.current_idx)
				self.current_idx++

				item := ordereddict.NewDict()

				// We failed to unmarshal one line of
				// JSON - it may be corrupted, go to
				// the next one.
				err = item.UnmarshalJSON(row_data)
				if err != nil {
					continue
				}

				output_chan <- TimelineItem{
					Row: item, Time: idx_record.Timestamp,
				}
			}
		}
	}()

	return output_chan

}

func (self *TimelineReader) Close() {
	self.fd.Close()
	self.index_fd.Close()
}

func NewTimelineReader(config_obj *config_proto.Config,
	path_manager *TimelinePathManager) (*TimelineReader, error) {
	file_store_factory := file_store.GetFileStore(config_obj)
	fd, err := file_store_factory.ReadFile(path_manager.Path())
	if err != nil {
		return nil, err
	}

	index_fd, err := file_store_factory.ReadFile(path_manager.Index())
	if err != nil {
		fd.Close()
		return nil, err
	}

	stats, err := index_fd.Stat()
	if err != nil {
		fd.Close()
		index_fd.Close()
		return nil, err
	}

	return &TimelineReader{fd: fd, index_fd: index_fd, index_stat: stats}, nil

}
