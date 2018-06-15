// An SQLite datastore.  Each client has its own database to avoid
// database contention.
package datastore

import (
	"database/sql"
	"errors"
	"fmt"
	"github.com/golang/protobuf/proto"
	_ "github.com/mattn/go-sqlite3"
	"path"
	"strings"
	"time"
	"www.velocidex.com/golang/velociraptor/config"
	crypto_proto "www.velocidex.com/golang/velociraptor/crypto/proto"
	"www.velocidex.com/golang/velociraptor/responder"
	utils "www.velocidex.com/golang/velociraptor/testing"
	"www.velocidex.com/golang/velociraptor/third_party/cache"
)

type SqliteDataStore struct {
	cache *cache.LRUCache
	clock utils.Clock
}

// Not thread safe - expects no other threads are using the connection
// right now.
func (self *SqliteDataStore) Close() {
	for _, item := range self.cache.Items() {
		item.Value.(CachedDB).handle.Close()
	}

	self.cache.Clear()
}

type CachedDB struct {
	handle *sql.DB
}

func (self CachedDB) Size() int {
	return 1
}

func (self *SqliteDataStore) getDB(db_path string) (*sql.DB, error) {
	handle, pres := self.cache.Get(db_path)
	if pres {
		return handle.(CachedDB).handle, nil

	} else {
		handle, err := sql.Open("sqlite3", db_path)
		if err != nil {
			return nil, err
		}
		self.cache.Set(db_path, CachedDB{handle})
		return handle, nil
	}
}

// Get the database that corresponds with the client id.
func getDBPathForClient(base string, client_id string) string {
	esaped_client_id := strings.Replace(
		strings.TrimPrefix(client_id, "aff4:/"),
		".", "%2E", 10)

	return path.Join(base, esaped_client_id+".sqlite")
}

func getDBPathForURN(base string, urn string) (string, error) {
	components := strings.Split(urn, "/")
	if len(components) <= 1 || components[0] != "aff4:" {
		return "", errors.New("Not an AFF4 Path")
	}

	if strings.HasPrefix(components[1], "C.") {
		return getDBPathForClient(base, components[1]), nil
	}

	return "", errors.New("Unknown URN mapping")
}

// Client queues hold messages for the client. NOTE: This is a naive
// queue implementation which can not handle concurrent access. This
// is adequate for the client queue because it does not see contention
// - each client is expected to drain its own queue on every poll, and
// we assume that the same client only has a single connection to the
// server at any one time.
func (self *SqliteDataStore) GetClientTasks(
	config *config.Config,
	client_id string) ([]*crypto_proto.GrrMessage, error) {
	var result []*crypto_proto.GrrMessage
	now := self.clock.Now().UTC().UnixNano()
	next_timestamp := self.clock.Now().Add(time.Second * 10).UTC().UnixNano()

	db_path := getDBPathForClient(*config.Datastore_location, client_id)
	handle, err := self.getDB(db_path)
	if err != nil {
		return nil, err
	}

	tasks_urn := path.Join("aff4:/", client_id, "tasks")

	// We lease tasks from the client queues and set their
	// timestamp into the future.
	update_statement, err := handle.Prepare(
		"update tbl set timestamp = ? where predicate = ?")
	if err != nil {
		return nil, err
	}
	defer update_statement.Close()

	rows, err := handle.Query(
		`select subject, predicate, timestamp, value from tbl
                 where subject = ? and timestamp < ?`, tasks_urn, now)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var predicates []string

	for rows.Next() {
		var subject string
		var predicate string
		var timestamp uint64
		var value []byte

		err := rows.Scan(&subject, &predicate, &timestamp, &value)
		if err != nil {
			return nil, err
		}
		message := &crypto_proto.GrrMessage{}
		err = proto.Unmarshal(value, message)
		if err == nil {
			result = append(result, message)
		}

		predicates = append(predicates, predicate)
	}

	for _, predicate := range predicates {
		update_statement.Exec(next_timestamp, predicate)
	}

	return result, nil
}

func (self *SqliteDataStore) RemoveTasksFromClientQueue(
	config_obj *config.Config,
	client_id string,
	task_ids []uint64) error {

	db_path := getDBPathForClient(*config_obj.Datastore_location, client_id)
	handle, err := self.getDB(db_path)
	if err != nil {
		return err
	}

	update_statement, err := handle.Prepare(
		"delete from tbl where predicate = ?")
	if err != nil {
		return err
	}
	defer update_statement.Close()

	for _, task_id := range task_ids {
		update_statement.Exec(fmt.Sprintf("task:%d", task_id))
	}

	return nil
}

func (self *SqliteDataStore) QueueMessageForClient(
	config_obj *config.Config,
	client_id string,
	flow_id string,
	client_action string,
	message proto.Message,
	next_state uint64) error {

	db_path := getDBPathForClient(*config_obj.Datastore_location, client_id)
	handle, err := self.getDB(db_path)
	if err != nil {
		return err
	}

	now := self.clock.Now().UTC().UnixNano()
	subject := fmt.Sprintf("aff4:/%s/%s", client_id, "tasks")
	predicate := fmt.Sprintf("task:%d", now)
	req, err := responder.NewRequest(message)
	if err != nil {
		return err
	}

	req.Name = client_action
	req.SessionId = flow_id
	req.RequestId = uint64(next_state)
	req.TaskId = uint64(now)

	value, err := proto.Marshal(req)
	if err != nil {
		return err
	}

	statement, err := handle.Prepare(
		`insert into tbl (subject, predicate, timestamp, value)
                   values (?, ?, ?, ?)`)
	if err != nil {
		return err
	}

	_, err = statement.Exec(
		subject, predicate, now, value)
	if err != nil {
		return err
	}

	return nil
}

func (self *SqliteDataStore) GetSubjectData(
	config_obj *config.Config,
	urn string) (map[string][]byte, error) {

	db_path, err := getDBPathForURN(*config_obj.Datastore_location, urn)
	if err != nil {
		return nil, err
	}
	handle, err := self.getDB(db_path)
	if err != nil {
		return nil, err
	}

	result := make(map[string][]byte)
	rows, err := handle.Query(
		`select predicate, value, max(timestamp) from tbl
                 where subject = ? group by subject, predicate`, urn)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	for rows.Next() {
		var predicate string
		var value []byte
		var timestamp uint64

		err := rows.Scan(&predicate, &value, &timestamp)
		if err != nil {
			return nil, err
		}
		result[predicate] = value
	}

	return result, nil
}

// Just grab the whole data of the AFF4 object.
func (self *SqliteDataStore) SetSubjectData(
	config_obj *config.Config,
	urn string,
	data map[string][]byte) error {

	utils.Debug(urn)

	db_path, err := getDBPathForURN(*config_obj.Datastore_location, urn)
	if err != nil {
		return err
	}

	handle, err := self.getDB(db_path)
	if err != nil {
		return err
	}

	now := self.clock.Now().UTC().UnixNano()

	statement, err := handle.Prepare(
		`insert into tbl (subject, predicate, timestamp, value)
                   values (?, ?, ?, ?)`)
	if err != nil {
		return err
	}

	for predicate, value := range data {
		_, err := statement.Exec(
			urn, predicate, now, value)
		if err != nil {
			return err
		}
	}

	return nil
}

func init() {
	db := SqliteDataStore{
		cache: cache.NewLRUCache(10),
		clock: utils.RealClock{},
	}

	RegisterImplementation("SqliteDataStore", &db)
}
