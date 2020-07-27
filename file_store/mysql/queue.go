// A Mysql queue implementation.

// The goal of this implementation is to allow for efficient following
// of various queues (queue name is the channel name we follow). Since
// Mysql does not have an event driven query we need to poll the
// queues periodically by quering for new rows. This query needs to be
// as efficient as possible.

// Since we also need to store the events in the client's monitoring
// space anyway we re-use the filestore table too. The filestore table
// looks like:

//     filestore(id int NOT NULL,
//              part int NOT NULL DEFAULT 0,
//              start_offset int,
//              end_offset int,
//              channel varchar,
//              timestamp TIMESTAMP DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
//              data blob,

// Normally we access the table using the id (corresponding to the
// filename) but the client's monitoring log may be written in
// different places. For queuing purposes, we use the channel column
// to denote the queue_name and the timestamp corresponding to the
// time the part was written.

// We then query for those data blobs that were written after the last
// poll time and have a channel we are interested in.

package mysql

import (
	"database/sql"
	"fmt"
	"sync"
	"time"

	"github.com/Velocidex/ordereddict"
	"github.com/golang/snappy"
	"www.velocidex.com/golang/velociraptor/file_store/api"
	"www.velocidex.com/golang/velociraptor/utils"
)

var (
	pool = NewQueuePool()
)

// A queue pool is an in-process listener for events.
type Listener struct {
	id      int64
	Channel chan *ordereddict.Dict
}

type QueuePool struct {
	mu sync.Mutex

	registrations map[string][]*Listener
	done          map[string]chan bool
}

func (self *QueuePool) Register(queue_name string) (<-chan *ordereddict.Dict, func()) {
	self.mu.Lock()
	defer self.mu.Unlock()

	registrations, pres := self.registrations[queue_name]
	if !pres {
		// There are no registrations yet, start a poller on
		// the queue.
		self.startPoller(queue_name)
	}
	new_registration := &Listener{
		Channel: make(chan *ordereddict.Dict, 1000),
		id:      time.Now().UnixNano(),
	}
	registrations = append(registrations, new_registration)

	self.registrations[queue_name] = registrations

	return new_registration.Channel, func() {
		self.unregister(queue_name, new_registration.id)
	}
}

func (self *QueuePool) startPoller(queue_name string) {
	done := make(chan bool)
	self.done[queue_name] = done

	go func() {
		// Keep track of the last part id
		last_id := sql.NullInt64{}

		for {
			err := self.emitEvents(queue_name, &last_id)
			if err != nil {
				return
			}

			select {
			case <-done:
				return

			case <-time.After(time.Second):
				continue
			}
		}
	}()
}

// Check for new events of this type.
func (self *QueuePool) emitEvents(queue_name string, last_id *sql.NullInt64) error {
	if !last_id.Valid {
		err := db.QueryRow("SELECT max(part_id) FROM filestore WHERE channel = ?",
			queue_name).Scan(last_id)
		if err != sql.ErrNoRows {
			// No entries exist yet. Start watching from 0.
			last_id.Valid = true
			return nil
		}
		return err
	}

	rows, err := db.Query(`
SELECT data, part_id
FROM filestore
WHERE channel = ? AND part_id > ?`, queue_name, last_id.Int64)
	if err != nil {
		fmt.Printf("Error: %v\n", err)
		return err
	}
	defer rows.Close()

	for rows.Next() {
		var data []byte
		err = rows.Scan(&data, last_id)
		if err != nil {
			continue
		}

		decoded, err := snappy.Decode(nil, data)
		if err != nil {
			continue
		}

		dict_rows, err := utils.ParseJsonToDicts(decoded)
		if err != nil {
			continue
		}

		for _, row := range dict_rows {
			self.Broadcast(queue_name, row)
		}
	}

	return nil
}

func (self *QueuePool) unregister(queue_name string, id int64) {
	self.mu.Lock()
	defer self.mu.Unlock()

	registrations, pres := self.registrations[queue_name]
	if pres {
		new_registrations := make([]*Listener, 0, len(registrations))
		for _, item := range registrations {
			if id == item.id {
				close(item.Channel)
			} else {
				new_registrations = append(new_registrations,
					item)
			}
		}

		if len(new_registrations) > 0 {
			self.registrations[queue_name] = new_registrations
			return
		}

		// Remove the registrations and stop the poller.
		delete(self.registrations, queue_name)
		done, pres := self.done[queue_name]
		if pres {
			close(done)
			delete(self.done, queue_name)
		}
	}
}

func (self *QueuePool) Broadcast(queue_name string, row *ordereddict.Dict) {
	self.mu.Lock()
	defer self.mu.Unlock()

	registrations, ok := self.registrations[queue_name]
	if ok {
		for _, item := range registrations {
			item.Channel <- row
		}
	}
}

func NewQueuePool() *QueuePool {
	return &QueuePool{
		registrations: make(map[string][]*Listener),
		done:          make(map[string]chan bool),
	}
}

type MysqlQueueManager struct {
	file_store *SqlFileStore
	Clock      utils.Clock
}

func (self *MysqlQueueManager) PushEventRows(
	path_manager api.PathManager, dict_rows []*ordereddict.Dict) error {

	if len(dict_rows) == 0 {
		return nil
	}

	for _, row := range dict_rows {
		// Set a timestamp per event for easier querying.
		row.Set("_ts", int(self.Clock.Now().Unix()))
	}

	log_path, err := path_manager.GetPathForWriting()
	if err != nil {
		return err
	}

	serialized, err := utils.DictsToJson(dict_rows, nil)
	if err != nil {
		return nil
	}

	fd, err := self.file_store.WriteFile(log_path)
	if err != nil {
		return err
	}

	// An internal write method to set the queue name in the table.
	_, err = fd.(*SqlWriter).write_row(path_manager.GetQueueName(), serialized)
	return err
}

func (self *MysqlQueueManager) Watch(queue_name string) (<-chan *ordereddict.Dict, func()) {
	return pool.Register(queue_name)
}

func NewMysqlQueueManager(file_store *SqlFileStore) api.QueueManager {
	return &MysqlQueueManager{
		file_store: file_store,
		Clock:      utils.RealClock{}}
}
