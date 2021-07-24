package timed

import (
	"context"
	"errors"
	"fmt"
	"io"
	"time"

	"github.com/Velocidex/ordereddict"
	"www.velocidex.com/golang/velociraptor/file_store/api"
	"www.velocidex.com/golang/velociraptor/result_sets"
	"www.velocidex.com/golang/velociraptor/timelines"
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

type partPathManager struct {
	log_path string
}

func (self partPathManager) GetPathForWriting() (string, error) {
	return self.log_path, nil
}

func (self partPathManager) GetQueueName() string {
	return ""
}

func (self partPathManager) GetAvailableFiles(
	ctx context.Context) []*api.ResultSetFileProperties {
	return nil
}

type TimedResultSetReader struct {
	files              []*api.ResultSetFileProperties
	current_files_idx  int
	current_reader     *timelines.TimelineReader
	start              time.Time
	end                time.Time
	file_store_factory api.FileStore
}

func (self *TimedResultSetReader) GetAvailableFiles(
	ctx context.Context) []*api.ResultSetFileProperties {
	return self.files
}

func (self *TimedResultSetReader) Debug() {
	fmt.Printf("Current idx %v\n", self.current_files_idx)
	for _, file := range self.files {
		fmt.Printf("%v %v-%v\n", file.Path, file.StartTime, file.EndTime)
	}
}

func (self *TimedResultSetReader) SeekToTime(offset time.Time) error {
	self.Close()

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
			if err == io.EOF {
				return nil
			}

			if err != nil {
				return err
			}
			err = reader.SeekToTime(offset)
			if err == io.EOF {
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
	if self.current_reader != nil {
		self.current_reader.Close()
		self.current_reader = nil
	}
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

		path_manager := timelinePathManager(current_file.Path)
		reader, err := timelines.NewTimelineReader(
			self.file_store_factory, path_manager)
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
	path_manager timelines.TimelinePathManagerInterface) (
	*timelines.TimelineReader, error) {

	reader, err := result_sets.NewResultSetReader(
		self.file_store_factory,
		partPathManager{path_manager.Path()})
	if err != nil {
		return nil, err
	}
	defer reader.Close()

	// Read all the lines from the json and write them to a new
	// tmp file.
	ctx := context.Background()
	new_path := path_manager.Path() + ".tmp"
	tmp_path_manager := timelinePathManager(new_path)
	tmp_writer, err := timelines.NewTimelineWriter(
		self.file_store_factory, tmp_path_manager,
		true /* truncate */)
	if err != nil {
		return nil, err
	}

	for row := range reader.Rows(ctx) {
		ts, pres := row.GetInt64("_ts")
		if pres {
			tmp_writer.Write(time.Unix(ts, 0), row)
		}
	}

	tmp_writer.Close()

	self.file_store_factory.Move(tmp_path_manager.Path(),
		path_manager.Path())
	self.file_store_factory.Move(tmp_path_manager.Index(),
		path_manager.Index())

	// Try to open the file again.
	return timelines.NewTimelineReader(
		self.file_store_factory, path_manager)
}

func (self *TimedResultSetReader) Rows(
	ctx context.Context) <-chan *ordereddict.Dict {
	output_chan := make(chan *ordereddict.Dict)

	go func() {
		defer close(output_chan)

		for {
			reader, err := self.getReader()
			if err != nil {
				return
			}

			for item := range reader.Read(ctx) {
				if !self.end.IsZero() &&
					item.Time.After(self.end) {
					break
				}

				select {
				case <-ctx.Done():
					return
				case output_chan <- item.Row:
				}
			}

			// When the reader is exhausted reset it so
			// next getReader() can pick the next reader.
			self.Close()
			self.current_files_idx++
		}
	}()

	return output_chan
}
