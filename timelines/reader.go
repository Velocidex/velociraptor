package timelines

import (
	"bufio"
	"context"
	"encoding/binary"
	"io"
	"os"
	"sort"
	"time"

	"github.com/Velocidex/ordereddict"
	ntfs "www.velocidex.com/golang/go-ntfs/parser"
	"www.velocidex.com/golang/velociraptor/file_store/api"
	"www.velocidex.com/golang/velociraptor/paths"
	timelines_proto "www.velocidex.com/golang/velociraptor/timelines/proto"
	"www.velocidex.com/golang/velociraptor/utils"
)

type TimelineItem struct {
	Row    *ordereddict.Dict
	Time   time.Time
	Source string
}

type TimelineReader struct {
	id                string
	current_idx       int
	offset            int64
	end_idx           int
	fd                api.FileReader
	index_fd          api.FileReader
	buffered_index_fd io.ReadSeeker
	index_stat        api.FileInfo
}

func (self *TimelineReader) getIndex(i int) (*IndexRecord, error) {
	// Reading past the end of the file is an EOF error
	if i > self.end_idx {
		return nil, io.EOF
	}

	idx_record := &IndexRecord{}
	_, err := self.buffered_index_fd.Seek(int64(i)*IndexRecordSize, 0)
	if err != nil {
		return nil, err
	}

	err = binary.Read(self.buffered_index_fd, binary.LittleEndian, idx_record)
	if err != nil {
		return nil, err
	}
	return idx_record, nil
}

func (self *TimelineReader) Stat() *timelines_proto.Timeline {
	first_record, _ := self.getIndex(0)
	last_record, _ := self.getIndex(int(self.index_stat.Size()/IndexRecordSize - 1))

	if first_record == nil || last_record == nil {
		return &timelines_proto.Timeline{Id: self.id}
	}

	return &timelines_proto.Timeline{
		Id:        self.id,
		StartTime: first_record.Timestamp,
		EndTime:   last_record.Timestamp,
	}
}

func (self *TimelineReader) SeekToTime(timestamp time.Time) error {
	timestamp_int := timestamp.UnixNano()
	number_of_points := self.index_stat.Size() / IndexRecordSize

	self.current_idx = sort.Search(int(number_of_points), func(i int) bool {
		// Read the index record at offset i
		idx_record, err := self.getIndex(i)
		if err != nil {
			return true
		}
		return idx_record.Timestamp >= timestamp_int
	})
	idx_record, err := self.getIndex(self.current_idx)
	if err != nil {
		return err
	}
	self.offset = idx_record.Offset
	return nil
}

func (self *TimelineReader) Read(ctx context.Context) <-chan TimelineItem {
	output_chan := make(chan TimelineItem)

	go func() {
		defer close(output_chan)

		self.fd.Seek(self.offset, os.SEEK_SET)
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

				idx_record, err := self.getIndex(self.current_idx)
				if err != nil {
					return
				}
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
					Source: self.id,
					Row:    item,
					Time:   time.Unix(0, idx_record.Timestamp),
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

func NewTimelineReader(
	file_store_factory api.FileStore,
	path_manager paths.TimelinePathManagerInterface) (*TimelineReader, error) {
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

	paged, err := ntfs.NewPagedReader(
		utils.MakeReaderAtter(index_fd), 1024*8, 10)
	if err != nil {
		return nil, err
	}

	return &TimelineReader{
		id:                path_manager.Name(),
		fd:                fd,
		index_fd:          index_fd,
		end_idx:           int(stats.Size()/IndexRecordSize - 1),
		buffered_index_fd: utils.NewReadSeekReaderAdapter(paged),
		index_stat:        stats,
	}, nil

}
