package timed

/*

Results can also be written directly as a timeline. This file
implements timeline writers and readers that can be used to directly
write result sets (i.e. rows) from queries.

*/

import (
	"errors"
	"time"

	"github.com/Velocidex/ordereddict"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	"www.velocidex.com/golang/velociraptor/file_store/api"
	"www.velocidex.com/golang/velociraptor/json"
	"www.velocidex.com/golang/velociraptor/paths"
	"www.velocidex.com/golang/velociraptor/result_sets"
	"www.velocidex.com/golang/velociraptor/timelines"
	"www.velocidex.com/golang/velociraptor/utils"
)

var (
	ignoreRowError = errors.New("Ignore log path")
)

type rowContainer struct {
	ts         time.Time
	serialized []byte
	count      int
}

type TimedResultSetWriterImpl struct {
	rows              []rowContainer
	total_rows_cached int

	opts         *json.EncOpts
	config_obj   *config_proto.Config
	path_manager api.PathManager

	// Recalculate the writer based on the log_path to support
	// correct file rotation.
	log_path      api.FSPathSpec
	last_log_base string
	writer        *timelines.TimelineWriter
	completer     *utils.Completer

	// Set for internal artifact writers to avoid writing to disk at all.
	internal bool
}

func (self *TimedResultSetWriterImpl) Write(row *ordereddict.Dict) {
	if self.internal {
		return
	}

	// Encode each row ASAP but then store the raw json for combined
	// writes. This allows us to get rid of memory from the query
	// ASAP.
	serialized, err := json.MarshalWithOptions(row, self.opts)
	if err != nil {
		return
	}

	self.rows = append(self.rows, rowContainer{
		serialized: serialized,
		count:      1,
		ts:         utils.Now(),
	})
	self.total_rows_cached += 1

	if self.total_rows_cached > 10000 {
		self.Flush()
	}
}

func (self *TimedResultSetWriterImpl) WriteJSONL(jsonl []byte, count int) {
	if self.internal {
		return
	}

	self.rows = append(self.rows, rowContainer{
		serialized: jsonl,
		count:      count,
		ts:         utils.Now(),
	})
	self.total_rows_cached += count

	if self.total_rows_cached > 10000 {
		self.Flush()
	}
}

// Do not actually write the data until Close() or Flush() are called,
// or until 10k rows are queued in memory.
func (self *TimedResultSetWriterImpl) Flush() {
	// Nothing to do...
	if self.total_rows_cached == 0 {
		return
	}

	for _, row := range self.rows {
		writer, err := self.getWriter(row.ts)
		if err == nil {
			_ = writer.WriteBuffer(row.ts, row.serialized)
		}
	}

	// Reset the slice.
	self.rows = self.rows[:0]
	self.total_rows_cached = 0
}

func (self *TimedResultSetWriterImpl) getWriter(ts time.Time) (
	*timelines.TimelineWriter, error) {
	log_path, err := self.path_manager.GetPathForWriting()
	if err != nil {
		return nil, err
	}

	// If no path is provided, we are just a log sink. This is used
	// for INTERNAL log writers.
	if log_path == nil {
		return nil, ignoreRowError
	}

	// The old writer is still fine for this time, just use it.
	if log_path.Base() == self.last_log_base && self.writer != nil {
		return self.writer, nil
	}

	writer, err := timelines.NewTimelineWriter(
		self.config_obj,
		paths.NewTimelinePathManager(
			log_path.Base(), log_path),
		self.completer.GetCompletionFunc(),
		result_sets.AppendMode)
	if err != nil {
		return nil, err
	}

	self.log_path = log_path
	self.last_log_base = log_path.Base()

	// Close the old writer and save the new one in its place.
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
		self.log_path = nil
		self.writer = nil
	}
}

func NewTimedResultSetWriter(
	config_obj *config_proto.Config,
	path_manager api.PathManager,
	opts *json.EncOpts,
	completion func()) (result_sets.TimedResultSetWriter, error) {

	log_path, err := path_manager.GetPathForWriting()
	if err != nil {
		return nil, err
	}

	return &TimedResultSetWriterImpl{
		config_obj:   config_obj,
		path_manager: path_manager,
		opts:         opts,

		// Only call the completion function once all writes
		// completed.
		completer: utils.NewCompleter(completion),
		internal:  log_path == nil,
	}, nil
}
