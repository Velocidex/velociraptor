package timed

import (
	"context"
	"errors"
	"fmt"
	"io"
	"sync"
	"time"

	"github.com/Velocidex/ordereddict"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	"www.velocidex.com/golang/velociraptor/file_store"
	"www.velocidex.com/golang/velociraptor/file_store/api"
	"www.velocidex.com/golang/velociraptor/paths"
	"www.velocidex.com/golang/velociraptor/result_sets"
	"www.velocidex.com/golang/velociraptor/timelines"
	"www.velocidex.com/golang/velociraptor/utils"
)

// Timed result sets are stored as regular result sets in rotated logs
// (by default rotating once per day).

// The Timed Result Sets are an abstraction built on top of this
// scheme which make the results set appear infinitely large, spanning
// arbitrary times. Callers can ask to start reading from a particular
// timestamp, and the abstraction find the relevant file it is in, and
// the relevant row.

// A TimedResultSetReader is constructed over the Timed result set
// consisting of a start time to an end time. Once constructed, the
// result set can be paged in a similar way to a regular result set,
// but it stitches rows from the different rotated log files as needed
// to cover the required time range.

// Internally we use a sequence of cursors to remember how to stitch
// rows from different result sets.  A cursor represents a point in
// time in the infinite timed result set, consisting of a JSONL log
// file, the row index in that file and the timestamp corresponding to
// that row. We also keep the total number of rows available in this
// log file part. We can start reading from this point simply by
// opening the result set and seeking to the row_idx and then reading
// from it, so reading from already constructed cursors is extremely
// quick.

type TimedResultSetReader struct {
	mu sync.Mutex

	files             []*api.ResultSetFileProperties
	current_files_idx int
	current_reader    *timelines.TimelineReader
	start             time.Time
	end               time.Time
	config_obj        *config_proto.Config
}

func (self *TimedResultSetReader) GetAvailableFiles(
	ctx context.Context) []*api.ResultSetFileProperties {
	self.mu.Lock()
	defer self.mu.Unlock()

	return append([]*api.ResultSetFileProperties{}, self.files...)
}

func (self *TimedResultSetReader) Debug() {
	self.mu.Lock()
	defer self.mu.Unlock()

	fmt.Printf("Current idx %v\n", self.current_files_idx)
	for _, file := range self.files {
		fmt.Printf("%v %v-%v\n", file.Path, file.StartTime, file.EndTime)
	}
}

func (self *TimedResultSetReader) SeekToTime(offset time.Time) error {
	self.mu.Lock()
	defer self.mu.Unlock()

	self._Close()

	self.start = offset
	for idx, file := range self.files {
		if offset.Before(file.StartTime) {
			self.current_files_idx = idx
			return nil
		}

		// This file spans the required time
		if (offset.Equal(file.StartTime) || offset.After(file.StartTime)) &&
			offset.Before(file.EndTime) {
			self.current_files_idx = idx

			reader, err := self.getReader()
			if errors.Is(err, io.EOF) {
				return nil
			}

			if err != nil {
				return err
			}

			err = reader.SeekToTime(offset)
			if errors.Is(err, io.EOF) {
				return nil
			}

			return err
		}
	}

	// No available time ranges found.
	self.current_files_idx = len(self.files)
	return nil
}

func (self *TimedResultSetReader) SetMaxTime(end time.Time) {
	self.end = end
}

func (self *TimedResultSetReader) Close() {
	self.mu.Lock()
	defer self.mu.Unlock()

	self._Close()
}

func (self *TimedResultSetReader) _Close() {
	if self.current_reader != nil {
		self.current_reader.Close()
		self.current_reader = nil
	}
}

func (self *TimedResultSetReader) GetReader() (*timelines.TimelineReader, error) {
	self.mu.Lock()
	defer self.mu.Unlock()

	return self.getReader()
}

func (self *TimedResultSetReader) getReader() (*timelines.TimelineReader, error) {
	if self.current_reader != nil {
		return self.current_reader, nil
	}

	// Search for the next file to open
	for self.current_files_idx < len(self.files) {
		current_file := self.files[self.current_files_idx]
		if !self.end.IsZero() &&
			current_file.StartTime.After(self.end) {
			return nil, io.EOF
		}

		reader_factory := timelines.TimelineReader{}

		path_manager := paths.NewTimelinePathManager(
			"", current_file.Path)
		reader, err := reader_factory.New(
			self.config_obj, timelines.UnitTransformer, path_manager)
		if err != nil {
			// Try to upgrade the index from older
			// versions.
			reader, err = self.maybeUpgradeIndex(path_manager)
			if err != nil {
				return nil, err
			}
		}

		self.current_reader = reader
		return reader, nil
	}

	return nil, errors.New("Not found")
}

func (self *TimedResultSetReader) maybeUpgradeIndex(
	path_manager paths.TimelinePathManagerInterface) (
	*timelines.TimelineReader, error) {

	file_store_factory := file_store.GetFileStore(self.config_obj)
	reader, err := result_sets.NewResultSetReader(
		file_store_factory, path_manager.Path())
	if err != nil {
		return nil, err
	}
	defer reader.Close()

	// Read all the lines from the json and write them to a new
	// tmp file.
	ctx := context.Background()
	new_path := path_manager.Path().
		SetType(api.PATH_TYPE_FILESTORE_TMP)
	tmp_path_manager := paths.NewTimelinePathManager("", new_path)

	// Write the tmp file synchronously and then read it again with
	// the benefit of the index.
	tmp_writer, err := timelines.NewTimelineWriter(
		self.config_obj, tmp_path_manager,
		utils.SyncCompleter, result_sets.TruncateMode)
	if err != nil {
		return nil, err
	}

	for row := range reader.Rows(ctx) {
		ts, pres := row.GetInt64("_ts")
		if pres {
			err := tmp_writer.Write(time.Unix(ts, 0), row)
			if err != nil {
				tmp_writer.Close()
				return nil, err
			}
		}
	}

	tmp_writer.Close()

	// Update the json file itself, and leave the new index
	// around.
	err = file_store_factory.Move(tmp_path_manager.Path(), path_manager.Path())
	if err != nil {
		return nil, err
	}

	reader_factory := timelines.TimelineReader{}

	// Try to open the file again.
	return reader_factory.New(
		self.config_obj, timelines.UnitTransformer, path_manager)
}

func (self *TimedResultSetReader) Rows(
	ctx context.Context) <-chan *ordereddict.Dict {
	output_chan := make(chan *ordereddict.Dict)

	go func() {
		defer close(output_chan)

		for {
			reader, err := self.GetReader()
			if err != nil {
				return
			}

			for item := range reader.Read(ctx) {
				if !self.end.IsZero() &&
					item.Time.After(self.end) {
					break
				}

				item.Row.Set("_ts", item.Time.UnixNano()/1000000)

				select {
				case <-ctx.Done():
					return
				case output_chan <- item.Row:
				}
			}

			// When the reader is exhausted reset it so
			// next GetReader() can pick the next reader.
			self.Close()

			self.mu.Lock()
			self.current_files_idx++
			self.mu.Unlock()
		}
	}()

	return output_chan
}
