package api

import (
	"context"

	"github.com/Velocidex/ordereddict"
)

// A QueueManager writes query results into queues. The manager is
// responsible for rotating the queue files as required.
type QueueManager interface {
	PushEventRows(path_manager PathManager, rows []*ordereddict.Dict) error
	Watch(queue_name string) (output <-chan *ordereddict.Dict, cancel func())
}

type ResultSetFileProperties struct {
	Path               string
	StartTime, EndTime int64
}

// Path manager tells the filestore where to store things.
type PathManager interface {
	// Gets a log path for writing new rows on.
	GetPathForWriting() (string, error)

	// The name of the queue we will use to watch for any rows
	// inserted into this result set.
	GetQueueName() string

	// Generate paths for reading linked result sets.
	GeneratePaths(ctx context.Context) <-chan *ResultSetFileProperties
}
