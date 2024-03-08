package api

import (
	"context"
	"time"

	"github.com/Velocidex/ordereddict"
)

type QueueOptions struct {
	DisableFileBuffering bool

	// How many items to lease at once from the file buffer.
	FileBufferLeaseSize int

	// Who is listening to this queue
	OwnerName string
}

// A QueueManager writes query results into queues. The manager is
// responsible for rotating the queue files as required.
type QueueManager interface {
	// Broadcast events only for local listeners without writing to
	// storage.
	Broadcast(path_manager PathManager, rows []*ordereddict.Dict)
	GetWatchers() []string

	PushEventRows(path_manager PathManager, rows []*ordereddict.Dict) error

	PushEventJsonl(path_manager PathManager, jsonl []byte, row_count int) error

	Watch(ctx context.Context, queue_name string, queue_options *QueueOptions) (
		output <-chan *ordereddict.Dict, cancel func())
}

type ResultSetFileProperties struct {
	Path               FSPathSpec
	StartTime, EndTime time.Time
	Size               int64
}

// Path manager tells the filestore where to store things.
type PathManager interface {
	// Gets a log path for writing new rows on.
	GetPathForWriting() (FSPathSpec, error)

	// The name of the queue we will use to watch for any rows
	// inserted into this result set.
	GetQueueName() string

	// Generate paths for reading linked result sets.
	GetAvailableFiles(ctx context.Context) []*ResultSetFileProperties
}
