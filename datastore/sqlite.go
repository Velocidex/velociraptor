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
	"www.velocidex.com/golang/velociraptor/constants"
	crypto_proto "www.velocidex.com/golang/velociraptor/crypto/proto"
	"www.velocidex.com/golang/velociraptor/responder"
	utils "www.velocidex.com/golang/velociraptor/testing"
	"www.velocidex.com/golang/velociraptor/third_party/cache"
)

const (
	LatestTime = 1
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

func runQuery(handle *sql.DB, query string) error {
	statement, err := handle.Prepare(query)
	if err != nil {
		return err
	}
	defer statement.Close()
	_, err = statement.Exec()

	return err
}

// Ensure the Database file is properly initialized.
func EnsureDB(handle *sql.DB) error {
	err := runQuery(handle, `CREATE TABLE IF NOT EXISTS tbl (
              subject TEXT NOT NULL,
              predicate TEXT NOT NULL,
              timestamp BIG INTEGER,
              value BLOB)`)
	if err != nil {
		return err
	}
	runQuery(handle,
		`CREATE UNIQUE INDEX subject_index ON tbl(subject, predicate)`)
	return nil
}

func (self *SqliteDataStore) getDB(db_path string) (*sql.DB, error) {
	handle, pres := self.cache.Get(db_path)
	if pres {
		return handle.(CachedDB).handle, nil

	} else {
		// journal_mode = WAL allows other processes to open
		// the DB at the same time.
		handle, err := sql.Open(
			"sqlite3", db_path+"?cache=shared&_journal_mode=WAL")

		if err != nil {
			return nil, err
		}
		err = EnsureDB(handle)
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

	if strings.HasPrefix(urn, constants.CLIENT_INDEX_URN) {
		return getDBPathForClient(base, components[1]), nil
	}

	return "", errors.New("Unknown URN mapping:" + urn)
}

// Client queues hold messages for the client. NOTE: This is a naive
// queue implementation which can not handle concurrent access. This
// is adequate for the client queue because it does not see contention
// - each client is expected to drain its own queue on every poll, and
// we assume that the same client only has a single connection to the
// server at any one time.
func (self *SqliteDataStore) GetClientTasks(
	config *config.Config,
	client_id string,
	do_not_lease bool) ([]*crypto_proto.GrrMessage, error) {
	var result []*crypto_proto.GrrMessage
	now := self.clock.Now().UTC().UnixNano() / 1000
	next_timestamp := self.clock.Now().Add(time.Second*10).UTC().UnixNano() / 1000

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

	if !do_not_lease {
		for _, predicate := range predicates {
			_, err := update_statement.Exec(next_timestamp, predicate)
			if err != nil {
				return nil, err
			}
		}
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
		_, err := update_statement.Exec(fmt.Sprintf("task:%d", task_id))
		if err != nil {
			return err
		}
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

	now := self.clock.Now().UTC().UnixNano() / 1000
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
	defer statement.Close()

	_, err = statement.Exec(
		subject, predicate, now, value)
	if err != nil {
		return err
	}

	return nil
}

func (self *SqliteDataStore) GetSubjectAttributes(
	config_obj *config.Config,
	urn string, attrs []string) (map[string][]byte, error) {
	db_path, err := getDBPathForURN(*config_obj.Datastore_location, urn)
	if err != nil {
		return nil, err
	}

	if len(attrs) == 0 {
		return nil, errors.New("Must provide some attributes")
	}

	handle, err := self.getDB(db_path)
	if err != nil {
		return nil, err
	}
	query := `select value, predicate, timestamp from tbl
                 where subject = ? and predicate in (?` +
		strings.Repeat(",?", len(attrs)-1) + ")"
	args := []interface{}{urn}
	for _, item := range attrs {
		args = append(args, item)
	}
	rows, err := handle.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	result := make(map[string][]byte)
	for rows.Next() {
		var value []byte
		var predicate string
		var timestamp uint64

		err := rows.Scan(&value, &predicate, &timestamp)
		if err != nil {
			return nil, err
		}
		result[predicate] = value
	}
	return result, nil
}

func (self *SqliteDataStore) GetSubjectData(
	config_obj *config.Config,
	urn string,
	offset uint64,
	count uint64) (map[string][]byte, error) {

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
		`select predicate, value, timestamp from tbl
                 where subject = ? order by predicate limit ?, ?`,
		urn, offset, count)
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
	urn string, timestamp int64,
	data map[string][]byte) error {

	db_path, err := getDBPathForURN(*config_obj.Datastore_location, urn)
	if err != nil {
		utils.Debug(err)
		return err
	}

	handle, err := self.getDB(db_path)
	if err != nil {
		utils.Debug(err)
		return err
	}

	if timestamp == LatestTime {
		timestamp = self.clock.Now().UTC().UnixNano() / 1000
	}
	statement, err := handle.Prepare(
		`insert or replace into tbl
                   (subject, predicate, timestamp, value) values (?, ?, ?, ?)`)
	if err != nil {
		utils.Debug(err)
		return err
	}
	defer statement.Close()

	for predicate, value := range data {
		_, err := statement.Exec(
			urn, predicate, timestamp, value)
		if err != nil {
			return err
		}
	}

	// Now update the index.
	statement2, err := handle.Prepare(
		`insert or replace into tbl
                   (subject, predicate, timestamp, value) values (?, ?, ?, ?)`)
	if err != nil {
		utils.Debug(err)
		return err
	}
	defer statement2.Close()

	// Note that insert or replace will update the previous
	// timestamp because the timestamp is not in the index.
	now := self.clock.Now().UTC().UnixNano() / 1000
	_, err = statement2.Exec(path.Dir(urn), "index:dir/"+path.Base(urn), now, "X")
	if err != nil {
		return err
	}

	return nil
}

func (self *SqliteDataStore) ListChildren(
	config_obj *config.Config,
	urn string,
	offset uint64, length uint64) ([]string, error) {
	db_path, err := getDBPathForURN(*config_obj.Datastore_location, urn)
	if err != nil {
		return nil, err
	}

	handle, err := self.getDB(db_path)
	if err != nil {
		return nil, err
	}

	rows, err := handle.Query(
		`select predicate from tbl where
                 subject = ? and
                 predicate like "index:dir/%" order by timestamp desc limit ?, ? `,
		urn, offset, length)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var result []string
	for rows.Next() {
		var predicate string
		err := rows.Scan(&predicate)
		if err != nil {
			return nil, err
		}
		result = append(result,
			path.Join(urn,
				strings.TrimPrefix(predicate, "index:dir/")))
	}

	return result, nil
}

func (self *SqliteDataStore) SetIndex(
	config_obj *config.Config,
	index_urn string,
	entity string,
	keywords []string) error {

	db_path, err := getDBPathForURN(*config_obj.Datastore_location, index_urn)
	if err != nil {
		return err
	}
	utils.Debug(db_path)
	handle, err := self.getDB(db_path)
	if err != nil {
		return err
	}

	statement, err := handle.Prepare(
		`insert or replace into tbl
                   (subject, predicate, timestamp, value) values (?, ?, ?, ?)`)
	if err != nil {
		return err
	}
	defer statement.Close()

	now := self.clock.Now().UTC().UnixNano() / 1000
	for _, keyword := range keywords {
		_, err := statement.Exec(
			path.Join(index_urn, strings.ToLower(keyword)),
			"kw_index:"+entity, now, "")
		if err != nil {
			return err
		}
	}

	return nil
}

func (self *SqliteDataStore) SearchClients(
	config_obj *config.Config,
	index_urn string,
	query string,
	offset uint64, limit uint64) []string {
	var result []string

	db_path, err := getDBPathForURN(*config_obj.Datastore_location, index_urn)
	if err != nil {
		return result
	}

	handle, err := self.getDB(db_path)
	if err != nil {
		return result
	}

	rows, err := handle.Query(
		`select predicate from tbl
                  where subject = ? group by predicate limit ?, ?`,
		path.Join(index_urn, query), offset, limit)

	if err != nil {
		return result
	}
	defer rows.Close()

	for rows.Next() {
		var client_id string

		err := rows.Scan(&client_id)
		if err != nil {
			return result
		}

		result = append(result, strings.TrimPrefix(client_id, "kw_index:"))
	}

	return result
}

func init() {
	db := SqliteDataStore{
		cache: cache.NewLRUCache(10),
		clock: utils.RealClock{},
	}

	RegisterImplementation("SqliteDataStore", &db)
}
