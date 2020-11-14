package result_sets

import (
	"context"
	"fmt"

	"github.com/Velocidex/ordereddict"
	"www.velocidex.com/golang/velociraptor/file_store/api"
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
type Cursor struct {
	Timestamp uint64
	RowIdx    uint64
	Filename  string

	// Total rows in this cursor with results.
	TotalRows uint64
}

// Return the total number of rows in a file.
func getNumberOfRowsInFile(
	ctx context.Context,
	file_store_factory api.FileStore,
	log_path string) uint64 {

	stat, err := file_store_factory.StatFile(log_path + ".index")
	if err == nil {
		return uint64(stat.Size()) / 8
	}

	// There is no index - fallback to the old slow way of
	// counting every line.
	fd, err := file_store_factory.ReadFile(log_path)
	if err != nil {
		return 0
	}
	defer fd.Close()

	// Brute force count all the rows in the file since there is
	// no index.
	count := uint64(0)
	rs_reader := &ResultSetReaderImpl{fd: fd}
	for range rs_reader.Rows(ctx) {
		count++
	}

	fmt.Printf("Brute force counted %v rows in %v\n", count, log_path)

	return count
}

// Find the cursor representing the timestamp
func GetCursors(ctx context.Context,
	file_store_factory api.FileStore,
	path_manager api.PathManager,
	start_time, end_time uint64) []*Cursor {

	result := []*Cursor{}

	var current_cursor *Cursor

	// Find the file that contains the timestamp
	for prop := range path_manager.GeneratePaths(ctx) {
		// Is this part entirely before the required range? If
		// so skip the entire part.
		if start_time > 0 && uint64(prop.EndTime) <= start_time {
			continue
		}

		// Have we found the start cursor? If so and this part
		// fits entirely inside the required range just add
		// the whole file.
		if len(result) > 0 && uint64(prop.EndTime) <= end_time {
			result = append(result, &Cursor{
				Timestamp: uint64(prop.StartTime),
				Filename:  prop.Path,
				TotalRows: getNumberOfRowsInFile(
					ctx, file_store_factory, prop.Path),
			})
			continue
		}

		// FIXME - for now this is not efficient - we open the
		// result set and read every row until we find the one
		// with a time larger than we need. This could be
		// improved by having a second timestamp index on the
		// result set.
		fd, err := file_store_factory.ReadFile(prop.Path)
		if err != nil {
			continue
		}
		defer fd.Close()

		count := uint64(0)
		defer func() {
			fmt.Printf("(%v, %v): Brute force counted %v rows in %v\n",
				start_time, end_time, count, prop)
		}()

		rs_reader := &ResultSetReaderImpl{fd: fd}
		for item := range rs_reader.Rows(ctx) {
			ts := uint64(utils.GetInt64(item, "_ts"))

			// If we have not found the start yet and this
			// row's timestamp is after the required
			// start, then we add a start cursor to it.
			if current_cursor == nil {
				// Row is earlier than required, skip it.
				if ts < start_time {
					count++
					continue
				}

				// Create a new cursor - it can
				// represent the rest of this part, or
				// a sub range within this part.
				current_cursor = &Cursor{
					Timestamp: uint64(ts),
					RowIdx:    count,
					Filename:  prop.Path,
				}

				// This part ends before the required
				// range so add it completely, and
				// go to the next file.
				if uint64(prop.EndTime) <= end_time {
					current_cursor.TotalRows = getNumberOfRowsInFile(
						ctx, file_store_factory, prop.Path) - count
					result = append(result, current_cursor)
					current_cursor = nil
					break
				}
			}

			// If the row timestamp exceeds the required
			// range and we are still working on a cursor,
			// then complete the cursor and finish this
			// function.
			if ts >= end_time {

				// We already started a cursor, finish
				// it and return.
				if current_cursor != nil {
					current_cursor.TotalRows = count - current_cursor.RowIdx
					result = append(result, current_cursor)
					return result
				}

				// Otherwise just add a new cursor
				// that covers this file up to the
				// current row.
				result = append(result, &Cursor{
					Timestamp: uint64(prop.StartTime),
					RowIdx:    0,
					Filename:  prop.Path,
					TotalRows: count,
				})
				return result
			}

			// Keep counting the rows until we exceed the
			// required range.
			count++
		}

		if current_cursor != nil {
			current_cursor.TotalRows = count
			result = append(result, current_cursor)
			current_cursor = nil
		}
	}

	return result
}

type partPathManager struct {
	log_path string
}

func (self partPathManager) GetPathForWriting() (string, error) {
	return self.log_path, nil
}

func (self partPathManager) GetQueueName() string {
	return ""
}

func (self partPathManager) GeneratePaths(
	ctx context.Context) <-chan *api.ResultSetFileProperties {
	return nil
}

type TimedResultSetReader struct {
	cursors            []*Cursor
	start_idx          uint64
	file_store_factory api.FileStore
}

func (self *TimedResultSetReader) SeekToRow(start int64) error {
	self.start_idx = uint64(start)
	return nil
}

func (self *TimedResultSetReader) Close() {}

// Total rows is just the sum of all rows in all the cursors.
func (self *TimedResultSetReader) TotalRows() int64 {
	result := int64(0)
	for _, cursor := range self.cursors {
		result += int64(cursor.TotalRows)
	}

	return result
}

func (self *TimedResultSetReader) Rows(ctx context.Context) <-chan *ordereddict.Dict {
	output_chan := make(chan *ordereddict.Dict)

	go func() {
		defer close(output_chan)

		skip_rows := self.start_idx

		// Iterate over all the cursors and generate all the
		// rows.
		for _, cursor := range self.cursors {
			// If this entire cursor is before the
			// required start index we skip it completely.
			if cursor.RowIdx+cursor.TotalRows < skip_rows {
				skip_rows -= cursor.RowIdx + cursor.TotalRows
				continue
			}

			path_manager := &partPathManager{cursor.Filename}
			rs_reader, err := factory.NewResultSetReader(
				self.file_store_factory, path_manager)
			if err != nil {
				return
			}

			// Skip the required number of rows in this cursor.
			part_index := cursor.RowIdx
			available_rows := cursor.TotalRows
			if skip_rows > 0 {
				// Take as many rows as possible from
				// skip_rows and give to part_index.
				to_skip := skip_rows
				if to_skip > available_rows {
					to_skip = available_rows
				}

				part_index += to_skip
				available_rows -= to_skip
				skip_rows -= to_skip
			}

			if available_rows <= 0 {
				continue
			}

			// Seek to the row we need.
			err = rs_reader.SeekToRow(int64(part_index))
			if err != nil {
				return
			}

			// The number of rows available in this part.
			for row := range rs_reader.Rows(ctx) {
				output_chan <- row
				if available_rows <= 0 {
					break
				}
				available_rows--
			}
		}
	}()

	return output_chan
}

func (self ResultSetFactory) NewTimedResultSetReader(
	ctx context.Context,
	file_store_factory api.FileStore,
	path_manager api.PathManager,
	start_time, end_time uint64) (ResultSetReader, error) {

	// Get the initial cursor
	cursors := GetCursors(ctx, file_store_factory, path_manager,
		start_time, end_time)

	return &TimedResultSetReader{
		cursors:            cursors,
		file_store_factory: file_store_factory}, nil
}
