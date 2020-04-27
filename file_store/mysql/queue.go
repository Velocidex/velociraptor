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
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/Velocidex/ordereddict"
	"github.com/golang/snappy"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	"www.velocidex.com/golang/velociraptor/file_store/api"
	"www.velocidex.com/golang/velociraptor/utils"
	"www.velocidex.com/golang/vfilter"
)

type MysqlQueueManager struct {
	file_store *SqlFileStore
	scope      *vfilter.Scope
	config_obj *config_proto.Config
}

func (self *MysqlQueueManager) PushEventRows(
	path_manager api.PathManager, source string,
	dict_rows []*ordereddict.Dict) error {

	if len(dict_rows) == 0 {
		return nil
	}

	log_path, err := path_manager.GetPathForWriting()
	if err != nil {
		return err
	}

	serialized, err := utils.DictsToJson(dict_rows)
	if err != nil {
		return nil
	}

	fd, err := self.file_store.WriteFile(log_path)
	if err != nil {
		return err
	}

	// An internal write method to set the queue name in the table.
	_, err = fd.(*SqlWriter).write_row(path_manager.GetArtifact(), serialized)
	return err
}

func (self *MysqlQueueManager) Watch(queue_name string) (<-chan *ordereddict.Dict, func()) {
	output := make(chan *ordereddict.Dict, 1000)
	ctx, cancel := context.WithCancel(context.Background())

	// Keep track of the last part id
	last_id := sql.NullInt64{}

	go func() {
		defer close(output)
		defer cancel()

		for {
			err := self.emitEvents(queue_name, &last_id, output)
			if err != nil {
				return
			}

			select {
			case <-ctx.Done():
				return

			case <-time.After(time.Second):
				continue
			}
		}
	}()

	return output, cancel
}

func (self *MysqlQueueManager) emitEvents(queue_name string,
	last_id *sql.NullInt64, output chan *ordereddict.Dict) error {

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

	fmt.Printf("Checking %v > %v\n", queue_name, last_id.Int64)

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
			output <- row
		}
	}

	return nil
}

func NewMysqlQueueManager(
	config_obj *config_proto.Config, file_store *SqlFileStore) (api.QueueManager, error) {
	return &MysqlQueueManager{
		file_store,
		vfilter.NewScope(),
		config_obj}, nil
}
