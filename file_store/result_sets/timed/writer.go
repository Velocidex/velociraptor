package timed

/*

Results can also be written directly as a timeline. This file
implements timeline writers and readers that can be used to directly
write result sets (i.e. rows) from queries.

*/

import (
	"errors"
	"time"

	"github.com/Velocidex/json"
	"github.com/Velocidex/ordereddict"
	"www.velocidex.com/golang/velociraptor/file_store/api"
	"www.velocidex.com/golang/velociraptor/file_store/result_sets"
	"www.velocidex.com/golang/velociraptor/timelines"
	"www.velocidex.com/golang/velociraptor/utils"
)

var (
	ignoreRowError = errors.New("Ignore log path")
)

type rowContainer struct {
	ts  time.Time
	row *ordereddict.Dict
}

type TimedResultSetWriterImpl struct {
	rows               []rowContainer
	opts               *json.EncOpts
	file_store_factory api.FileStore

	path_manager api.PathManager

	// Recalculate the writer based on the log_path to support
	// correct file rotation.
	log_path string
	writer   *timelines.TimelineWriter

	Clock utils.Clock
}

func (self *TimedResultSetWriterImpl) Write(row *ordereddict.Dict) {
	self.rows = append(self.rows, rowContainer{
		row: row,
		ts:  self.Clock.Now(),
	})

	if len(self.rows) > 10000 {
		self.Flush()
	}
}

// Do not actually write the data until Close() or Flush() are called,
// or until 10k rows are queued in memory.
func (self *TimedResultSetWriterImpl) Flush() {
	// Nothing to do...
	if len(self.rows) == 0 {
		return
	}

	for _, row := range self.rows {
		writer, err := self.getWriter(row.ts)
		if err == nil {
			writer.Write(row.ts, row.row)
		}
	}

	// Reset the slice.
	self.rows = self.rows[:0]
}

func (self *TimedResultSetWriterImpl) getWriter(ts time.Time) (
	*timelines.TimelineWriter, error) {
	log_path, err := self.path_manager.GetPathForWriting()
	if err != nil {
		return nil, err
	}

	// If no path is provided, we are just a log sink
	if log_path == "" {
		return nil, ignoreRowError
	}

	if log_path == self.log_path {
		return self.writer, nil
	}

	writer, err := timelines.NewTimelineWriter(self.file_store_factory,
		timelinePathManager(log_path))
	if err != nil {
		return nil, err
	}

	self.log_path = log_path
	if self.writer != nil {
		self.writer.Close()
	}
	self.writer = writer

	return self.writer, nil
}

func (self *TimedResultSetWriterImpl) Close() {
	self.Flush()
	if self.writer != nil {
		self.writer.Close()
		self.log_path = ""
		self.writer = nil
	}
}

type timelinePathManager string

func (self timelinePathManager) Path() string {
	return string(self)
}

func (self timelinePathManager) Name() string {
	return string(self)
}

func (self timelinePathManager) Index() string {
	return string(self) + ".index"
}

func NewTimedResultSetWriter(
	file_store_factory api.FileStore,
	path_manager api.PathManager,
	opts *json.EncOpts) (result_sets.TimedResultSetWriter, error) {

	return &TimedResultSetWriterImpl{
		file_store_factory: file_store_factory,
		path_manager:       path_manager,
		opts:               opts,
		Clock:              utils.RealClock{},
	}, nil
}
