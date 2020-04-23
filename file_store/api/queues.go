package api

import (
	"time"

	"github.com/Velocidex/ordereddict"
	"www.velocidex.com/golang/vfilter"
)

// A QueueManager writes query results into queues. The manager is
// responsible for rotating the queue files as required.
type QueueManager interface {
	Push(queue_name, source string, mode int, rows []byte) error
	PushRow(queue_name, source string, mode int, row *ordereddict.Dict) error
	Read(queue_name, source string, start_time, endtime time.Time) <-chan vfilter.Row
	Watch(queue_name string) (output <-chan *ordereddict.Dict, cancel func())
}
