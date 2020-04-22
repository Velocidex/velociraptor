package api

import (
	"time"

	"www.velocidex.com/golang/vfilter"
)

// A QueueManager writes query results into queues. The manager is
// responsible for rotating the queue files as required.
type QueueManager interface {
	Push(queue_name, source string, rows []byte) error
	Read(queue_name, source string, start_time, endtime time.Time) <-chan vfilter.Row
	Watch(queue_name, source string) <-chan vfilter.Row
}
