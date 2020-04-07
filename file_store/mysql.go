package file_store

import (
	"context"
	"crypto/sha1"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"sync"
	"time"

	"github.com/pkg/errors"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	"www.velocidex.com/golang/velociraptor/glob"
	"www.velocidex.com/golang/velociraptor/third_party/cache"
	"www.velocidex.com/golang/velociraptor/utils"
)

var (
	my_sql_mu             sync.Mutex
	db_initialized        bool
	set_subject_dir_cache *cache.LRUCache
)

const (
	MAX_BLOB_SIZE = 1<<16 - 1
)

type _cache_item struct{}

func (self _cache_item) Size() int { return 1 }

type MysqlFileStoreFileInfo struct {
	path      string
	name      string
	is_dir    bool
	size      int64
	timestamp int64
}

func (self MysqlFileStoreFileInfo) FullPath() string {
	return path.Join(self.path, self.name)
}

func (self *MysqlFileStoreFileInfo) Mtime() glob.TimeVal {
	return glob.TimeVal{
		Sec:  self.timestamp,
		Nsec: self.timestamp * 1000000000,
	}
}

func (self MysqlFileStoreFileInfo) Atime() glob.TimeVal {
	return glob.TimeVal{}
}

func (self MysqlFileStoreFileInfo) Ctime() glob.TimeVal {
	return glob.TimeVal{
		Sec:  self.timestamp,
		Nsec: self.timestamp * 1000000000,
	}
}

func (self MysqlFileStoreFileInfo) Data() interface{} {
	return nil
}

func (self MysqlFileStoreFileInfo) IsLink() bool {
	return false
}

func (self MysqlFileStoreFileInfo) GetLink() (string, error) {
	return "", errors.New("Not implemented")
}

func (self MysqlFileStoreFileInfo) Name() string {
	return self.name
}

func (self MysqlFileStoreFileInfo) Size() int64 {
	return self.size
}

func (self MysqlFileStoreFileInfo) Mode() os.FileMode {
	result := os.FileMode(0777)
	if self.is_dir {
		result |= os.ModeDir
	}
	return result
}

func (self MysqlFileStoreFileInfo) ModTime() time.Time {
	return time.Unix(self.timestamp, 0)
}

func (self MysqlFileStoreFileInfo) IsDir() bool {
	if self.size > 0 {
		return false
	}
	return self.is_dir
}

func (self MysqlFileStoreFileInfo) Sys() interface{} {
	return nil
}

func (self *MysqlFileStoreFileInfo) MarshalJSON() ([]byte, error) {
	result, err := json.Marshal(&struct {
		FullPath string
		Size     int64
		Mode     os.FileMode
		ModeStr  string
		ModTime  time.Time
		Sys      interface{}
		Mtime    glob.TimeVal
		Ctime    glob.TimeVal
		Atime    glob.TimeVal
	}{
		FullPath: self.FullPath(),
		Size:     self.Size(),
		Mode:     self.Mode(),
		ModeStr:  self.Mode().String(),
		ModTime:  self.ModTime(),
		Sys:      self.Sys(),
		Mtime:    self.Mtime(),
		Ctime:    self.Ctime(),
		Atime:    self.Atime(),
	})

	return result, err
}

func (self *MysqlFileStoreFileInfo) UnmarshalJSON(data []byte) error {
	return nil
}

type SqlReader struct {
	config_obj *config_proto.Config

	offset        int64
	current_chunk []byte
	part          int64
	file_id       int64
	filename      string

	is_dir    bool
	size      int64
	timestamp int64
}

// Seek loads a new chunk into the current_chunk buffer and prepares
// for further reading.
func (self *SqlReader) Seek(offset int64, whence int) (int64, error) {
	// This is basically a tell.
	if offset == 0 && whence == os.SEEK_CUR {
		return self.offset, nil
	}

	if whence != os.SEEK_SET {
		panic(fmt.Sprintf("Unsupported seek on %v (%v %v)!",
			self.filename, offset, whence))
	}

	// Find which part contains the required offset.
	db, err := sql.Open("mysql", self.config_obj.Datastore.MysqlConnectionString)
	if err != nil {
		return 0, err
	}
	defer db.Close()

	start_offset := int64(0)

	// This query tries to find the part which covers the required
	// offset. The inner query uses the (id, start_offset) index
	// to find 1 row which has the start_offset just below the
	// offset. We then
	err = db.QueryRow(`
SELECT A.part, A.start_offset, A.data FROM (
    SELECT part FROM filestore WHERE id=? AND start_offset <= ?
    ORDER BY start_offset DESC LIMIT 1
) AS B
JOIN filestore as A
ON A.part = B.part AND A.id = ? AND A.end_offset > ?`,
		self.file_id, offset, self.file_id, offset).Scan(
		&self.part, &start_offset, &self.current_chunk)

	// No valid range is found we are seeking past end of file.
	if err == sql.ErrNoRows {
		self.part = -1
		self.offset = offset
		self.current_chunk = nil
		return offset, nil
	}

	// Some other error happened.
	if err != nil {
		return 0, err
	}

	// The part is covered.
	self.current_chunk = self.current_chunk[offset-start_offset:]

	return offset, nil
}

func (self SqlReader) Close() error {
	return nil
}

func (self *SqlReader) Read(buff []byte) (int, error) {
	// Reading out of bound.
	if self.part < 0 {
		return 0, io.EOF
	}

	db, err := sql.Open("mysql", self.config_obj.Datastore.MysqlConnectionString)
	if err != nil {
		return 0, err
	}
	defer db.Close()

	offset := 0
	for offset < len(buff) {
		// Drain the current chunk until is it empty.
		if len(self.current_chunk) > 0 {
			n := copy(buff[offset:], self.current_chunk)
			self.current_chunk = self.current_chunk[n:]
			offset += n
			continue
		}

		// Get the next chunk and cache it.
		blob := []byte{}
		err := db.QueryRow(`SELECT data from filestore WHERE id=? AND part = ?`,
			self.file_id, self.part).Scan(&blob)

		// No more parts available right now.
		if err == sql.ErrNoRows {
			break
		}

		if err != nil {
			return offset, err
		}

		self.current_chunk = append([]byte{}, blob...)

		// An empty chunk means no more data.
		if len(self.current_chunk) == 0 {
			break
		}

		// Next chunk id
		self.part += 1
	}

	self.offset += int64(offset)
	if offset == 0 {
		return 0, io.EOF
	}

	return offset, nil
}

func (self SqlReader) Stat() (glob.FileInfo, error) {
	dir_path, name := utils.PathSplit(self.filename)

	return &MysqlFileStoreFileInfo{
		path:      dir_path,
		name:      name,
		is_dir:    self.is_dir,
		size:      self.size,
		timestamp: self.timestamp,
	}, nil

	return nil, errors.New("Not Implemented")
}

type SqlWriter struct {
	config_obj *config_proto.Config
	file_id    int64
	size       int64
	filename   string
}

func (self SqlWriter) Size() (int64, error) {
	return self.size, nil
}

func (self SqlWriter) Close() error {
	return nil
}

func (self *SqlWriter) Write(buff []byte) (int, error) {
	if len(buff) == 0 {
		return 0, nil
	}

	db, err := sql.Open("mysql", self.config_obj.Datastore.MysqlConnectionString)
	if err != nil {
		return 0, err
	}
	defer db.Close()

	// TODO - retry transaction.
	ctx := context.Background()
	tx, err := db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelSerializable})
	if err != nil {
		return 0, err
	}

	defer tx.Rollback()

	last_part_query, err := tx.Prepare(`
SELECT A.part, A.end_offset FROM filestore AS A join (
   SELECT max(part) AS max_part FROM filestore WHERE id=?
) AS B
ON A.part = B.max_part AND A.id = ?`)
	if err != nil {
		return 0, err
	}
	defer last_part_query.Close()

	insert, err := tx.Prepare(`
INSERT INTO filestore (id, part, start_offset, end_offset, data) VALUES (?, ?, ?, ?,?)`)
	if err != nil {
		return 0, err
	}
	defer insert.Close()

	update_metadata, err := tx.Prepare(`
UPDATE filestore_metadata SET timestamp=now(), size=size + ? WHERE id = ?`)
	if err != nil {
		return 0, err
	}
	defer update_metadata.Close()

	// Append the buffer to the data table
	end := int64(0)
	part := int64(0)

	// Get the last part and end offset. SELECT max() on a primary
	// key is instant. We then use this part to look up the row
	// using the all_column index.
	err = last_part_query.QueryRow(self.file_id, self.file_id).Scan(
		&part, &end)
	// No parts exist yet
	if err == sql.ErrNoRows {
		part = 0
	} else if err != nil {
		return 0, err
	} else {
		part += 1
	}

	total_length := int64(0)

	// Push the buffer into the table one chunk at the time.
	for len(buff) > 0 {
		// We store the data in blobs which are limited to
		// 64kb.
		length := int64(len(buff))
		if length > MAX_BLOB_SIZE {
			length = MAX_BLOB_SIZE
		}

		// Write this chunk only.
		chunk := buff[:length]

		_, err = insert.Exec(
			self.file_id, part, end, end+length, chunk)
		if err != nil {
			return 0, err
		}

		// Increase our size
		self.size = end + length
		total_length += length

		// Advance the buffer some more.
		buff = buff[length:]
		part += 1
	}

	_, err = update_metadata.Exec(total_length, self.file_id)
	if err != nil {
		return 0, err
	}

	err = tx.Commit()
	if err != nil {
		return 0, err
	}

	return len(buff), nil
}

func (self SqlWriter) Truncate() error {
	db, err := sql.Open("mysql", self.config_obj.Datastore.MysqlConnectionString)
	if err != nil {
		return err
	}
	defer db.Close()

	// TODO - retry transaction.
	ctx := context.Background()
	tx, err := db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelSerializable})
	if err != nil {
		return err
	}

	// Essentially delete all the filestore rows for this file id.
	_, err = tx.Exec("DELETE FROM filestore WHERE id = ?", self.file_id)
	if err != nil {
		_ = tx.Rollback()
		return err
	}

	// Reset the size of the file in the metadata.
	_, err = tx.Exec(`UPDATE filestore_metadata SET timestamp=now(), size=0 WHERE id = ?`,
		self.file_id)
	if err != nil {
		_ = tx.Rollback()
		return err
	}

	err = tx.Commit()
	if err != nil {
		return err
	}

	return err
}

func hash(path string) string {
	hash := sha1.Sum([]byte(path))
	return string(hash[:])
}

type SqlFileStore struct {
	config_obj *config_proto.Config
}

func (self *SqlFileStore) ReadFile(filename string) (FileReader, error) {
	result := &SqlReader{
		config_obj: self.config_obj,
		filename:   filename,
	}

	db, err := sql.Open("mysql", self.config_obj.Datastore.MysqlConnectionString)
	if err != nil {
		return nil, err
	}
	defer db.Close()

	dir_path, name := utils.PathSplit(filename)

	// Create the file metadata
	err = db.QueryRow(`
SELECT id, size, is_dir, unix_timestamp(timestamp)
FROM filestore_metadata WHERE path_hash = ? AND name = ?`,
		hash(dir_path), name).Scan(
		&result.file_id, &result.size,
		&result.is_dir, &result.timestamp)

	if err == sql.ErrNoRows {
		return nil, os.ErrNotExist
	}

	if err != nil {
		return nil, err
	}

	return result, nil
}

func makeDirs(components []string, db *sql.DB) error {
	for len(components) > 0 {
		dir_path := utils.JoinComponents(components[:len(components)-1], "/")
		name := components[len(components)-1]

		_, err := db.Exec(`
INSERT IGNORE INTO filestore_metadata (path, path_hash, name, is_dir) values(?, ?, ?, true)`,
			dir_path, hash(dir_path), name)
		if err != nil {
			return err
		}
		components = components[:len(components)-1]
	}
	return nil
}

func (self *SqlFileStore) WriteFile(filename string) (FileWriter, error) {
	last_id := int64(0)
	size := int64(0)

	components := utils.SplitComponents(filename)
	if len(components) > 0 {
		dir_path := utils.JoinComponents(components[:len(components)-1], "/")
		name := components[len(components)-1]

		db, err := sql.Open("mysql", self.config_obj.Datastore.MysqlConnectionString)
		if err != nil {
			return nil, err
		}
		defer db.Close()

		ctx := context.Background()
		tx, err := db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelSerializable})
		if err != nil {
			return nil, err
		}
		defer tx.Rollback()

		err = tx.QueryRow(`
SELECT id, size FROM filestore_metadata WHERE path_hash =? and name = ?`,
			hash(dir_path), name).Scan(&last_id, &size)
		if err == sql.ErrNoRows {
			// Create the file metadata
			res, err := tx.Exec(`
INSERT INTO filestore_metadata (path, path_hash, name, is_dir) values(?, ?, ?, false)`,
				dir_path, hash(dir_path), name)
			if err != nil {
				return nil, err
			}

			last_id, err = res.LastInsertId()
			if err != nil {
				return nil, err
			}
		} else if err != nil {
			return nil, err
		}

		err = tx.Commit()
		if err != nil {
			return nil, err
		}

		err = makeDirs(components, db)
		if err != nil {
			return nil, err
		}

	}

	return &SqlWriter{
		config_obj: self.config_obj,
		file_id:    last_id,
		filename:   filename,
		size:       size,
	}, nil
}

func (self *SqlFileStore) StatFile(filename string) (os.FileInfo, error) {
	return nil, errors.New("Not Implemented")
}

func (self *SqlFileStore) ListDirectory(dirname string) ([]os.FileInfo, error) {
	result := []os.FileInfo{}
	db, err := sql.Open("mysql", self.config_obj.Datastore.MysqlConnectionString)
	if err != nil {
		return nil, err
	}
	defer db.Close()

	components := utils.SplitComponents(dirname)
	dir_name := utils.JoinComponents(components, "/")

	rows, err := db.Query(`
SELECT path, name, unix_timestamp(timestamp), size, is_dir
FROM filestore_metadata
WHERE path_hash = ? AND path = ?`, hash(dir_name), dir_name)
	if err != nil {
		return nil, err
	}

	for rows.Next() {
		row := &MysqlFileStoreFileInfo{}
		err = rows.Scan(&row.path, &row.name, &row.timestamp, &row.size,
			&row.is_dir)
		if err != nil {
			return nil, err
		}

		result = append(result, row)
	}

	return result, nil
}

func (self *SqlFileStore) Walk(root string, walkFn filepath.WalkFunc) error {
	children, err := self.ListDirectory(root)
	if err != nil {
		return err
	}

	for _, child_info := range children {
		full_path := path.Join(root, child_info.Name())
		err1 := walkFn(full_path, child_info, err)
		if err1 == filepath.SkipDir {
			continue
		}

		err1 = self.Walk(full_path, walkFn)
		if err1 != nil {
			return err1
		}
	}

	return nil
}

func (self *SqlFileStore) Delete(filename string) error {
	components := utils.SplitComponents(filename)
	if len(components) > 0 {
		db, err := sql.Open("mysql", self.config_obj.Datastore.MysqlConnectionString)
		if err != nil {
			return err
		}
		defer db.Close()

		dir_path := utils.JoinComponents(components[:len(components)-1], "/")
		name := components[len(components)-1]

		ctx := context.Background()
		tx, err := db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelSerializable})
		if err != nil {
			return err
		}
		defer tx.Rollback()

		id := 0
		err = tx.QueryRow(`
SELECT id FROM filestore_metadata WHERE path_hash =? and name = ?`,
			hash(dir_path), name).Scan(&id)

		// The file does not actually exist - its not an error.
		if err == sql.ErrNoRows {
			return nil
		}

		// Delete the file metadata and file data.
		_, err = tx.Exec(`DELETE FROM filestore WHERE id = ?`, id)
		if err != nil {
			return err
		}

		_, err = tx.Exec(`DELETE FROM filestore_metadata WHERE id = ?`, id)
		if err != nil {
			return err
		}

		return tx.Commit()
	}

	return nil
}

func (self *SqlFileStore) Get(filename string) ([]byte, bool) {
	return nil, false
}

func (self *SqlFileStore) Clear() {}

func NewSqlFileStore(config_obj *config_proto.Config) (FileStore, error) {
	my_sql_mu.Lock()
	defer my_sql_mu.Unlock()

	if !db_initialized {
		err := initializeDatabase(
			config_obj, config_obj.Datastore.MysqlConnectionString,
			config_obj.Datastore.MysqlDatabase)
		if err != nil {
			return nil, err
		}
		db_initialized = true
	}

	return &SqlFileStore{config_obj: config_obj}, nil
}

func initializeDatabase(
	config_obj *config_proto.Config,
	database_connection_string, database string) error {

	db, err := sql.Open("mysql", database_connection_string)
	if err != nil {
		return err
	}
	defer db.Close()

	// If specifying the connection string we assume the database
	// already exists.
	if database != "" {
		// If the database does not exist we need to connect
		// to a blank database to issue the create database.
		conn_string := fmt.Sprintf("%s:%s@tcp(%s)/",
			config_obj.Datastore.MysqlUsername,
			config_obj.Datastore.MysqlPassword,
			config_obj.Datastore.MysqlServer)
		db, err := sql.Open("mysql", conn_string)
		if err != nil {
			return err
		}
		defer db.Close()

		_, err = db.Exec(fmt.Sprintf("create database if not exists `%v`",
			database))
		if err != nil {
			return err
		}
	}

	_, err = db.Exec(`create table if not exists
    filestore(id int NOT NULL,
              part int NOT NULL DEFAULT 0,
              start_offset int,
              end_offset int,
              data blob,
              INDEX(id, part, start_offset, end_offset),
              INDEX(id, start_offset))`)
	if err != nil {
		return err
	}

	_, err = db.Exec(`create table if not exists
    filestore_metadata(
              id INT NOT NULL AUTO_INCREMENT,
              path text,
              path_hash BLOB(20),
              name varchar(256),
              timestamp TIMESTAMP DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
              size int NOT NULL DEFAULT 0,
              is_dir bool NOT NULL DEFAULT false,
              PRIMARY KEY (id),
              INDEX(path_hash(20)),
              unique INDEX(path_hash(20), name))`)
	if err != nil {
		return err
	}

	return nil
}

type SqlFileStoreAccessor struct {
	file_store *SqlFileStore
}

func (self SqlFileStoreAccessor) New(ctx context.Context) glob.FileSystemAccessor {
	return &SqlFileStoreAccessor{self.file_store}
}

func (self *SqlFileStoreAccessor) Lstat(filename string) (glob.FileInfo, error) {
	lstat, err := self.file_store.StatFile(filename)
	if err != nil {
		return nil, err
	}

	return &MysqlFileStoreFileInfo{path: filename,
		name:      lstat.Name(),
		is_dir:    lstat.IsDir(),
		size:      lstat.Size(),
		timestamp: lstat.ModTime().Unix(),
	}, err
}

func (self *SqlFileStoreAccessor) ReadDir(path string) ([]glob.FileInfo, error) {
	files, err := self.file_store.ListDirectory(path)
	if err != nil {
		return nil, err
	}

	result := []glob.FileInfo{}
	for _, item := range files {
		result = append(result, item.(*MysqlFileStoreFileInfo))
	}
	return result, nil
}

func (self *SqlFileStoreAccessor) Open(path string) (glob.ReadSeekCloser, error) {
	fd, err := self.file_store.ReadFile(path)
	if err != nil {
		return nil, err
	}
	return &FileReaderAdapter{fd}, nil
}

var SqlFileStoreAccessor_re = regexp.MustCompile("/")

func (self SqlFileStoreAccessor) PathSplit(path string) []string {
	return SqlFileStoreAccessor_re.Split(path, -1)
}

func (self SqlFileStoreAccessor) PathJoin(root, stem string) string {
	return path.Join(root, stem)
}

func (self *SqlFileStoreAccessor) GetRoot(path string) (string, string, error) {
	return "/", path, nil
}

func init() {
	set_subject_dir_cache = cache.NewLRUCache(1000000)
}
