/*
   Velociraptor - Hunting Evil
   Copyright (C) 2019 Velocidex Innovations.

   This program is free software: you can redistribute it and/or modify
   it under the terms of the GNU Affero General Public License as published
   by the Free Software Foundation, either version 3 of the License, or
   (at your option) any later version.

   This program is distributed in the hope that it will be useful,
   but WITHOUT ANY WARRANTY; without even the implied warranty of
   MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
   GNU Affero General Public License for more details.

   You should have received a copy of the GNU Affero General Public License
   along with this program.  If not, see <https://www.gnu.org/licenses/>.
*/
// +build sqlite

// An SQLite datastore.  Each client has its own database to avoid
// database contention.

// This used to be the default data store but now the default is
// FileBaseDataStore.
package datastore

import (
	"database/sql"
	"fmt"
	"github.com/golang/protobuf/proto"
	_ "github.com/mattn/go-sqlite3"
	errors "github.com/pkg/errors"
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
		return errors.WithStack(err)
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
			return nil, errors.WithStack(err)
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

// Route the different URNs to their own SQLite database.
func getDBPathForURN(base string, urn string) (string, error) {
	components := strings.Split(urn, "/")
	if len(components) <= 1 || components[0] != "aff4:" {
		return "", errors.New("Not an AFF4 Path: " + urn)
	}

	if strings.HasPrefix(components[1], "C.") {
		return getDBPathForClient(base, components[1]), nil
	}

	if strings.HasPrefix(urn, constants.CLIENT_INDEX_URN) {
		return getDBPathForClient(base, components[1]), nil
	}

	if strings.HasPrefix(urn, constants.HUNTS_URN) {
		return getDBPathForClient(base, "hunts"), nil
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
	next_timestamp := self.clock.Now().Add(
		time.Second*time.Duration(config.Frontend.ClientLeaseTime)).
		UTC().UnixNano() / 1000

	db_path := getDBPathForClient(config.Datastore.Location, client_id)
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
		return nil, errors.WithStack(err)
	}
	defer update_statement.Close()

	rows, err := handle.Query(
		`select subject, predicate, timestamp, value from tbl
                 where subject = ? and timestamp < ?`, tasks_urn, now)
	if err != nil {
		return nil, errors.WithStack(err)
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
			return nil, errors.WithStack(err)
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
				return nil, errors.WithStack(err)
			}
		}
	}

	return result, nil
}

func (self *SqliteDataStore) RemoveTasksFromClientQueue(
	config_obj *config.Config,
	client_id string,
	task_ids []uint64) error {

	db_path := getDBPathForClient(config_obj.Datastore.Location, client_id)
	handle, err := self.getDB(db_path)
	if err != nil {
		return err
	}

	update_statement, err := handle.Prepare(
		"delete from tbl where predicate = ?")
	if err != nil {
		return errors.WithStack(err)
	}
	defer update_statement.Close()

	for _, task_id := range task_ids {
		_, err := update_statement.Exec(fmt.Sprintf("task:%d", task_id))
		if err != nil {
			return errors.WithStack(err)
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

	db_path := getDBPathForClient(config_obj.Datastore.Location, client_id)
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
		return errors.WithStack(err)
	}

	statement, err := handle.Prepare(
		`insert into tbl (subject, predicate, timestamp, value)
                   values (?, ?, ?, ?)`)
	if err != nil {
		return errors.WithStack(err)
	}
	defer statement.Close()

	_, err = statement.Exec(
		subject, predicate, now, value)
	if err != nil {
		return errors.WithStack(err)
	}

	return nil
}

func (self *SqliteDataStore) GetSubject(
	config_obj *config.Config,
	urn string,
	message proto.Message) error {

	db_path, err := getDBPathForURN(config_obj.Datastore.Location, urn)
	if err != nil {
		return err
	}
	handle, err := self.getDB(db_path)
	if err != nil {
		return err
	}

	rows, err := handle.Query(
		`select value, timestamp from tbl
                 where subject = ? and predicate = ?`,
		urn, constants.AFF4_ATTR)
	if err != nil {
		return errors.WithStack(err)
	}
	defer rows.Close()

	for rows.Next() {
		var value []byte
		var timestamp uint64

		err := rows.Scan(&value, &timestamp)
		if err != nil {
			return errors.WithStack(err)
		}
		err = proto.Unmarshal(value, message)
		if err != nil {
			return errors.WithStack(err)
		}
	}

	return nil
}

func (self *SqliteDataStore) SetSubject(
	config_obj *config.Config,
	urn string,
	message proto.Message) error {
	serialized_content, err := proto.Marshal(message)
	if err != nil {
		return errors.WithStack(err)
	}

	db_path, err := getDBPathForURN(config_obj.Datastore.Location, urn)
	if err != nil {
		return err
	}

	handle, err := self.getDB(db_path)
	if err != nil {
		utils.Debug(err)
		return err
	}

	timestamp := self.clock.Now().UTC().UnixNano() / 1000
	statement, err := handle.Prepare(
		`insert or replace into tbl
                   (subject, predicate, timestamp, value) values (?, ?, ?, ?)`)
	if err != nil {
		return errors.WithStack(err)
	}
	defer statement.Close()

	_, err = statement.Exec(urn, constants.AFF4_ATTR,
		timestamp, serialized_content)
	if err != nil {
		return errors.WithStack(err)
	}

	// Now update the index.
	statement2, err := handle.Prepare(
		`insert or replace into tbl
                   (subject, predicate, timestamp, value) values (?, ?, ?, ?)`)
	if err != nil {
		return errors.WithStack(err)
	}
	defer statement2.Close()

	// Note that insert or replace will update the previous
	// timestamp because the timestamp is not in the index.
	now := self.clock.Now().UTC().UnixNano() / 1000
	_, err = statement2.Exec(path.Dir(urn), "index:dir/"+path.Base(urn), now, "X")
	if err != nil {
		return errors.WithStack(err)
	}

	return nil
}

func (self *SqliteDataStore) DeleteSubject(
	config_obj *config.Config,
	urn string) error {
	db_path, err := getDBPathForURN(config_obj.Datastore.Location, urn)
	if err != nil {
		return err
	}

	handle, err := self.getDB(db_path)
	if err != nil {
		return err
	}

	statement, err := handle.Prepare(
		"delete from tbl where subject = ?")
	if err != nil {
		return errors.WithStack(err)
	}
	defer statement.Close()

	_, err = statement.Exec(urn)
	if err != nil {
		return errors.WithStack(err)
	}

	// Also remove the directory index.
	statement2, err := handle.Prepare(
		"delete from tbl where subject = ? and predicate = ?")
	if err != nil {
		return errors.WithStack(err)
	}
	defer statement2.Close()

	_, err = statement2.Exec(path.Dir(urn), "index:dir/"+path.Base(urn))
	if err != nil {
		return errors.WithStack(err)
	}

	return nil
}

func (self *SqliteDataStore) ListChildren(
	config_obj *config.Config,
	urn string,
	offset uint64, length uint64) ([]string, error) {
	db_path, err := getDBPathForURN(config_obj.Datastore.Location, urn)
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
		return nil, errors.WithStack(err)
	}
	defer rows.Close()

	var result []string
	for rows.Next() {
		var predicate string
		err := rows.Scan(&predicate)
		if err != nil {
			return nil, errors.WithStack(err)
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

	db_path, err := getDBPathForURN(config_obj.Datastore.Location, index_urn)
	if err != nil {
		return err
	}
	handle, err := self.getDB(db_path)
	if err != nil {
		return err
	}

	statement, err := handle.Prepare(
		`insert or replace into tbl
                   (subject, predicate, timestamp, value) values (?, ?, ?, ?)`)
	if err != nil {
		return errors.WithStack(err)
	}
	defer statement.Close()

	now := self.clock.Now().UTC().UnixNano() / 1000
	for _, keyword := range keywords {
		_, err := statement.Exec(
			path.Join(index_urn, strings.ToLower(keyword)),
			"kw_index:"+entity, now, "")
		if err != nil {
			return errors.WithStack(err)
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

	db_path, err := getDBPathForURN(config_obj.Datastore.Location, index_urn)
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
