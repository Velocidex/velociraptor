package datastore

import (
	"crypto/sha1"
	"database/sql"
	"fmt"
	"path"
	"strings"
	"sync"

	_ "github.com/go-sql-driver/mysql"

	"github.com/golang/protobuf/jsonpb"
	"github.com/golang/protobuf/proto"
	errors "github.com/pkg/errors"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	crypto_proto "www.velocidex.com/golang/velociraptor/crypto/proto"
	"www.velocidex.com/golang/velociraptor/testing"
	"www.velocidex.com/golang/velociraptor/third_party/cache"
	"www.velocidex.com/golang/velociraptor/utils"
)

var (
	mu sync.Mutex

	// Global db handle.
	db                    *sql.DB
	set_subject_dir_cache *cache.LRUCache
)

type _cache_item int

func (self _cache_item) Size() int { return 1 }

type DataStoreRow struct {
	Path      string
	PathHash  []byte
	Name      string
	Timestamp int64
	Data      []byte
}

type MySQLDataStore struct {
	FileBaseDataStore
}

func (self *MySQLDataStore) GetSubject(
	config_obj *config_proto.Config,
	urn string,
	message proto.Message) error {

	serialized_content, err := readContentToMysqlRow(
		config_obj, urn, false /* must_exist */)
	if err != nil {
		fmt.Printf("GetSubject: %v %v\n", urn, err)
		return err
	}

	if strings.HasSuffix(urn, ".json") {
		return jsonpb.UnmarshalString(
			string(serialized_content), message)
	}

	err = proto.Unmarshal(serialized_content, message)
	if err != nil {
		fmt.Printf("GetSubject: %v %v\n", urn, err)
	}
	return err
}

func (self *MySQLDataStore) SetSubject(
	config_obj *config_proto.Config,
	urn string,
	message proto.Message) error {

	// Encode as JSON
	if strings.HasSuffix(urn, ".json") {
		marshaler := &jsonpb.Marshaler{Indent: " "}
		serialized_content, err := marshaler.MarshalToString(
			message)
		if err != nil {
			return err
		}
		return writeContentToMysqlRow(
			config_obj, urn, []byte(serialized_content))
	}
	serialized_content, err := proto.Marshal(message)
	if err != nil {
		return errors.WithStack(err)
	}

	return writeContentToMysqlRow(config_obj, urn, serialized_content)
}

func (self *MySQLDataStore) DeleteSubject(
	config_obj *config_proto.Config,
	urn string) error {

	dir_path, name := utils.PathSplit(urn)
	hash := sha1.Sum([]byte(dir_path))

	_, err := db.Exec(`
DELETE FROM datastore WHERE path =?  AND  path_hash = ? and  name = ?`,
		dir_path, string(hash[:]), name)
	if err != nil {
		return err
	}

	return nil
}

// Lists all the children of a URN.
func (self *MySQLDataStore) ListChildren(
	config_obj *config_proto.Config,
	urn string,
	offset uint64, length uint64) ([]string, error) {

	children, err := self.listChildren(config_obj, urn, offset, length)
	if err != nil {
		return nil, err
	}

	// ListChildren returns the full URN
	result := make([]string, 0, len(children))
	for _, child := range children {
		result = append(result, utils.PathJoin(urn, child, "/"))
	}

	return result, nil
}

// Returns only the children
func (self *MySQLDataStore) listChildren(
	config_obj *config_proto.Config,
	urn string,
	offset uint64, length uint64) ([]string, error) {

	// In the database directories do not contain a trailing /
	components := utils.SplitComponents(urn)
	urn = utils.JoinComponents(components, "/")

	hash := sha1.Sum([]byte(urn))
	rows, err := db.Query(`
SELECT name FROM datastore WHERE path =? AND path_hash = ?
ORDER BY timestamp DESC LIMIT ?, ?`,
		urn, string(hash[:]), offset, length)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	results := []string{}
	for rows.Next() {
		row := &DataStoreRow{}
		err = rows.Scan(&row.Name)
		if err != nil {
			return nil, err
		}
		results = append(results, row.Name)
	}

	return results, nil
}

// Update the posting list index. Searching for any of the
// keywords will return the entity urn.
func (self *MySQLDataStore) SetIndex(
	config_obj *config_proto.Config,
	index_urn string,
	entity string,
	keywords []string) error {
	for _, keyword := range keywords {
		subject := path.Join(index_urn, strings.ToLower(keyword), entity)
		err := self.SetSubject(config_obj, subject,
			&crypto_proto.GrrMessage{RequestId: 1})
		if err != nil {
			return err
		}
	}
	return nil
}

func (self *MySQLDataStore) UnsetIndex(
	config_obj *config_proto.Config,
	index_urn string,
	entity string,
	keywords []string) error {

	for _, keyword := range keywords {
		subject := path.Join(index_urn, strings.ToLower(keyword), entity)
		err := self.DeleteSubject(config_obj, subject)
		if err != nil {
			return err
		}
	}
	return nil

}

func (self *MySQLDataStore) CheckIndex(
	config_obj *config_proto.Config,
	index_urn string,
	entity string,
	keywords []string) error {
	data := &crypto_proto.GrrMessage{}

	for _, keyword := range keywords {
		subject := path.Join(index_urn, strings.ToLower(keyword), entity)
		err := self.GetSubject(config_obj, subject, data)
		// Any of the matching entities means we checked
		// successfully.
		if err == nil && data.RequestId == 1 {
			return nil
		}
	}
	return errors.New("Client does not have label")
}

func (self *MySQLDataStore) SearchClients(
	config_obj *config_proto.Config,
	index_urn string,
	query string, query_type string,
	offset uint64, limit uint64) []string {
	seen := make(map[string]bool)
	result := []string{}

	query = strings.ToLower(query)
	if query == "." || query == "" {
		query = "all"
	}

	add_func := func(key string) {
		children, err := self.listChildren(config_obj,
			path.Join(index_urn, key), 0, 1000)
		if err != nil {
			return
		}
		for _, name := range children {
			seen[name] = true

			if uint64(len(seen)) > offset+limit {
				break
			}
		}
	}

	// Query has a wildcard.
	if strings.ContainsAny(query, "[]*?") {
		// We could make it smarter in future but this is
		// quick enough for now.
		sets, err := self.listChildren(
			config_obj, index_urn, 0, 1000)
		if err != nil {
			return result
		}

		for _, name := range sets {
			matched, err := path.Match(query, name)
			if err != nil {
				// Can only happen if pattern is invalid.
				return result
			}
			if matched {
				if query_type == "key" {
					seen[name] = true
				} else {
					add_func(name)
				}
			}

			if uint64(len(seen)) > offset+limit {
				break
			}
		}
	} else {
		add_func(query)
	}

	for k := range seen {
		result = append(result, k)
	}

	if uint64(len(result)) < offset {
		return []string{}
	}

	if uint64(len(result))-offset < limit {
		limit = uint64(len(result)) - offset
	}

	return result[offset : offset+limit]
}

// Called to close all db handles etc. Not thread safe.
func (self *MySQLDataStore) Close() {}

func NewMySQLDataStore(config_obj *config_proto.Config) (DataStore, error) {
	mu.Lock()
	defer mu.Unlock()

	if db == nil {
		var err error
		db, err = initializeDatabase(config_obj)
		if err != nil {
			return nil, err
		}
	}

	return &MySQLDataStore{FileBaseDataStore{clock: testing.RealClock{}}}, nil
}

func initializeDatabase(
	config_obj *config_proto.Config) (*sql.DB, error) {

	db, err := sql.Open("mysql", config_obj.Datastore.MysqlConnectionString)
	if err != nil {
		return nil, err
	}
	// Deliberatly do not close db as it is a global.

	// If specifying the connection string we assume the database
	// already exists.
	if config_obj.Datastore.MysqlDatabase != "" {
		// If the database does not exist we need to connect
		// to a blank database to issue the create database.
		conn_string := fmt.Sprintf("%s:%s@tcp(%s)/",
			config_obj.Datastore.MysqlUsername,
			config_obj.Datastore.MysqlPassword,
			config_obj.Datastore.MysqlServer)
		db, err := sql.Open("mysql", conn_string)
		if err != nil {
			return nil, err
		}
		defer db.Close()

		_, err = db.Exec(fmt.Sprintf("create database if not exists `%v`",
			config_obj.Datastore.MysqlDatabase))
		if err != nil {
			return nil, err
		}
	}

	_, err = db.Exec(`create table if not exists
    datastore(id int NOT NULL AUTO_INCREMENT PRIMARY KEY,
              path text,
              path_hash BLOB(20),
              name varchar(256),
              timestamp TIMESTAMP DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
              data blob,
              INDEX(path_hash(20)), unique INDEX(path_hash(20), name))`)
	if err != nil {
		return nil, err
	}

	return db, nil
}

func writeContentToMysqlRow(
	config_obj *config_proto.Config,
	urn string,
	serialized_content []byte) error {
	db, err := sql.Open("mysql", config_obj.Datastore.MysqlConnectionString)
	if err != nil {
		return err
	}
	defer db.Close()

	components := utils.SplitComponents(urn)
	for len(components) > 0 {
		dir_path := utils.JoinComponents(components[:len(components)-1], "/")
		name := components[len(components)-1]
		hash_array := sha1.Sum([]byte(dir_path))
		hash := string(hash_array[:])

		if serialized_content != nil {
			_, err := db.Exec(`
REPLACE INTO datastore (path, path_hash, name, data) VALUES (?, ?, ?, ?)`,
				dir_path, hash, name, serialized_content)
			if err != nil {
				return err
			}

			// If we just want to touch directories we do
			// not want to over write existing rows
		} else {
			_, err := db.Exec(`
INSERT IGNORE INTO datastore (path, path_hash, name) VALUES (?, ?, ?)`,
				dir_path, hash, name)
			if err != nil {
				return err
			}
		}

		// Stay in the loop until all sub directories are
		// touched.
		_, ok := set_subject_dir_cache.Get(hash)
		if ok {
			return nil
		}
		set_subject_dir_cache.Set(hash, _cache_item(0))

		components = components[:len(components)-1]
		serialized_content = nil
	}

	return nil
}

func readContentToMysqlRow(
	config_obj *config_proto.Config,
	urn string,
	must_exist bool) ([]byte, error) {

	db, err := sql.Open("mysql", config_obj.Datastore.MysqlConnectionString)
	if err != nil {
		return nil, err
	}
	defer db.Close()

	dir_path, name := utils.PathSplit(urn)
	hash := sha1.Sum([]byte(dir_path))

	rows, err := db.Query(`
SELECT data FROM datastore WHERE path_hash = ? AND path = ? AND name = ? LIMIT 1`,
		string(hash[:]), dir_path, name)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	for rows.Next() {
		row := &DataStoreRow{}

		err = rows.Scan(&row.Data)
		if err != nil {
			return nil, err
		}
		return row.Data, nil
	}

	if must_exist {
		return nil, errors.New("Not found")
	}

	return nil, nil
}

func init() {
	set_subject_dir_cache = cache.NewLRUCache(1000000)
}
