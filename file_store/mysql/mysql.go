package mysql

import (
	"context"
	"crypto/sha1"
	"database/sql"
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"sync"
	"time"

	"www.velocidex.com/golang/velociraptor/json"

	"github.com/Velocidex/ordereddict"
	_ "github.com/go-sql-driver/mysql"
	"github.com/golang/snappy"
	"github.com/pkg/errors"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	"www.velocidex.com/golang/velociraptor/file_store/api"
	"www.velocidex.com/golang/velociraptor/glob"
	"www.velocidex.com/golang/velociraptor/utils"
	"www.velocidex.com/golang/vfilter"
)

/*
The MySql data store relies on two tables:

filestore_metadata:
+-----+----------------------------------------------------+---------------------------+---------------------+----------+--------+
| id  | path                                               | name                      | timestamp           | size     | is_dir |
+-----+----------------------------------------------------+---------------------------+---------------------+----------+--------+
|   3 | /                                                  | public                    | 2020-04-08 16:33:53 |        0 |      1 |
|  84 | /server_artifacts/Server.Monitor.Health/2020-04-08 | Prometheus.csv            | 2020-04-08 17:53:47 |     5155 |      0 |
+-----+----------------------------------------------------+---------------------------+---------------------+----------+--------+

filestore:
+-----+------+--------------+------------+--------------+
| id  | part | start_offset | end_offset | length(data) |
+-----+------+--------------+------------+--------------+
| 719 |    0 |            0 |          0 |         NULL |
| 719 |    1 |            0 |      64512 |        40339 |
| 719 |    2 |        64512 |     129024 |        41815 |
| 719 |    3 |       129024 |     193536 |        42395 |
+-----+------+--------------+------------+--------------+

The 0'th row is a place holder for empty files. The real parts start
at part id 1.

Insertion- New chunks are added automatically to the filestore table
in an atomic insert/select query. This means that inserts may occur by
multiple writers to the same table at the same time. This will result
in interleaving of data on the same file.

Since Velociraptor simply writers CSV files, and each write operation
contains a whole number of CSV rows this allows multiple writers to
stream their results to the same file without needing to lock access
to the database.

*/

var (
	my_sql_mu sync.Mutex

	// A global DB object - initialized once and reused. According
	//  to http://go-database-sql.org/accessing.html Although it’s
	//  idiomatic to Close() the database when you’re finished
	//  with it, the sql.DB object is designed to be
	//  long-lived. Don’t Open() and Close() databases
	//  frequently. Instead, create one sql.DB object for each
	//  distinct datastore you need to access, and keep it until
	//  the program is done accessing that datastore. Pass it
	//  around as needed, or make it available somehow globally,
	//  but keep it open. And don’t Open() and Close() from a
	//  short-lived function. Instead, pass the sql.DB into that
	//  short-lived function as an argument.
	db *sql.DB
)

const (
	// Allow a bit of overheads for snappy compression.
	MAX_BLOB_SIZE = 1<<16 - 1024
)

type MysqlFileStoreFileInfo struct {
	path      string
	name      string
	is_dir    bool
	size      int64
	timestamp int64
	id        int64
}

func (self MysqlFileStoreFileInfo) FullPath() string {
	return path.Join(self.path, self.name)
}

func (self *MysqlFileStoreFileInfo) Mtime() utils.TimeVal {
	return utils.TimeVal{
		Sec:  self.timestamp,
		Nsec: self.timestamp * 1000000000,
	}
}

func (self MysqlFileStoreFileInfo) Atime() utils.TimeVal {
	return utils.TimeVal{}
}

func (self MysqlFileStoreFileInfo) Ctime() utils.TimeVal {
	return utils.TimeVal{
		Sec:  self.timestamp,
		Nsec: self.timestamp * 1000000000,
	}
}

func (self MysqlFileStoreFileInfo) Data() interface{} {
	return ordereddict.NewDict().
		Set("id", self.id)
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
		Mtime    utils.TimeVal
		Ctime    utils.TimeVal
		Atime    utils.TimeVal
		Data     interface{}
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
		Data:     self.Data(),
	})

	return result, err
}

func (self *MysqlFileStoreFileInfo) UnmarshalJSON(data []byte) error {
	return nil
}

type SqlReader struct {
	config_obj *config_proto.Config

	// Offset within the current cached chunk
	offset        int64
	current_chunk []byte
	// Current part and part offset
	part        int64
	part_offset int64
	file_id     int64
	filename    string

	is_dir    bool
	size      int64
	timestamp int64
}

// Seek loads a new chunk into the current_chunk buffer and prepares
// for further reading.
func (self *SqlReader) Seek(offset int64, whence int) (int64, error) {
	// This is basically a tell.
	if offset == 0 && whence == os.SEEK_CUR {
		return self.part_offset + self.offset, nil
	}

	if whence != io.SeekStart {
		panic(fmt.Sprintf("Unsupported seek on %v (%v %v)!",
			self.filename, offset, whence))
	}

	// This query tries to find the part which covers the required
	// offset. The inner query uses the (id, start_offset) index
	// to find 1 row which has the start_offset just below the
	// offset. We then
	blob := []byte{}

	err := db.QueryRow(`
SELECT A.part, A.start_offset, A.data FROM (
    SELECT part FROM filestore WHERE id=? AND start_offset <= ?
    ORDER BY end_offset DESC LIMIT 1
) AS B
JOIN filestore as A
ON A.part = B.part AND A.id = ? AND A.end_offset > ? AND A.end_offset != A.start_offset`,
		self.file_id, offset, self.file_id, offset).Scan(
		&self.part, &self.part_offset, &blob)

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

	self.current_chunk, err = snappy.Decode(nil, blob)
	if err != nil {
		return 0, err
	}

	// Offset within the chunk
	self.offset = offset - self.part_offset

	// The offset is past the end of the chunk
	if int(self.offset) > len(self.current_chunk) {
		return 0, errors.New("Bad chunk")
	}

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

	if self.current_chunk == nil {
		_, err := self.Seek(0, io.SeekStart)
		if err != nil {
			return 0, err
		}
	}

	offset := 0
	for offset < len(buff) && len(self.current_chunk) > 0 {
		// Drain the current chunk until is it empty.
		if len(self.current_chunk) > int(self.offset) {
			n := copy(buff[offset:], self.current_chunk[self.offset:])
			offset += n
			self.offset += int64(n)
			continue
		}

		// Get the next chunk and cache it.
		blob := []byte{}
		err := db.QueryRow(`SELECT data from filestore WHERE id=? AND part = ?`,
			self.file_id, self.part+1).Scan(&blob)

		self.offset = 0

		// No more parts available right now.
		if err == sql.ErrNoRows {
			break
		}

		if err != nil {
			return offset, err
		}

		self.current_chunk, err = snappy.Decode(nil, blob)
		if err != nil {
			return offset, err
		}

		// Next chunk id
		self.part += 1
	}

	// We did not fill the buffer at all.
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
		id:        self.file_id,
	}, nil
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
	return self.write_row("", buff)
}

func (self *SqlWriter) write_row(channel string, buff []byte) (n int, err error) {
	if len(buff) == 0 {
		return 0, nil
	}

	// TODO - retry transaction.
	ctx := context.Background()
	tx, err := db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelSerializable})
	if err != nil {
		return 0, err
	}

	// Commit or rollback depending on error.
	defer func() {
		if err != nil {
			// Keep the original error
			_ = tx.Rollback()
			return
		}
		err = tx.Commit()
	}()

	insert, err := tx.Prepare(`
INSERT INTO filestore (id, part, start_offset, end_offset, data, channel)
SELECT A.id AS id,
       A.part + 1 AS part,
       A.end_offset AS start_offset,
       A.end_offset + ? AS end_offset,
       ? AS data,
       ? AS channel
FROM filestore AS A join (
   SELECT max(part) AS max_part FROM filestore WHERE id=?
) AS B
ON A.part = B.max_part AND A.id = ?`)
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
		chunk := snappy.Encode(nil, buff[:length])
		_, err = insert.Exec(length, chunk, channel,
			self.file_id, self.file_id)
		if err != nil {
			return 0, err
		}

		// Increase our size
		self.size += length
		total_length += length

		// Advance the buffer some more.
		buff = buff[length:]
	}

	_, err = update_metadata.Exec(total_length, self.file_id)
	if err != nil {
		return 0, err
	}

	return int(total_length), nil
}

func (self SqlWriter) Truncate() (err error) {
	// TODO - retry transaction.
	ctx := context.Background()
	tx, err := db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelSerializable})
	if err != nil {
		return err
	}
	// Commit or rollback depending on error.
	defer func() {
		if err != nil {
			// Keep the original error
			_ = tx.Rollback()
			return
		}
		err = tx.Commit()
	}()

	// Essentially delete all the filestore rows for this file id.
	_, err = tx.Exec("DELETE FROM filestore WHERE id = ? AND part != 0", self.file_id)
	if err != nil {
		return err
	}

	// Reset the size of the file in the metadata.
	_, err = tx.Exec(`UPDATE filestore_metadata SET timestamp=now(), size=0 WHERE id = ?`,
		self.file_id)
	if err != nil {
		return err
	}

	self.size = 0

	return nil
}

func hash(path string) string {
	hash := sha1.Sum([]byte(path))
	return string(hash[:])
}

type SqlFileStore struct {
	config_obj *config_proto.Config
}

func (self *SqlFileStore) ReadFile(filename string) (api.FileReader, error) {
	result := &SqlReader{
		config_obj: self.config_obj,
		filename:   filename,

		// Parts start counting at 1.
		part: 1,
	}

	dir_path, name := utils.PathSplit(filename)

	// Create the file metadata
	err := db.QueryRow(`
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

func (self *SqlFileStore) WriteFile(filename string) (r api.FileWriter, err error) {
	last_id := int64(0)
	size := int64(0)

	components := utils.SplitComponents(filename)
	if len(components) > 0 {
		dir_path := utils.JoinComponents(components[:len(components)-1], "/")
		name := components[len(components)-1]

		ctx := context.Background()
		tx, err := db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelSerializable})
		if err != nil {
			return nil, err
		}

		// Commit or rollback depending on error.
		defer func() {
			if err != nil {
				// Keep the original error
				_ = tx.Rollback()
				return
			}
			// We explicitely commit below.
		}()

		err = tx.QueryRow(`
SELECT id, size FROM filestore_metadata
WHERE path = ? AND path_hash =? and name = ?`,
			dir_path, hash(dir_path), name).Scan(&last_id, &size)
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
			_, err = tx.Exec(`
INSERT INTO filestore (id, part, start_offset, end_offset)
VALUES(?, 0, 0, 0)`, last_id)
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
	components := utils.SplitComponents(filename)
	dir_name := utils.JoinComponents(components[:len(components)-1], "/")
	base_name := components[len(components)-1]

	rows, err := db.Query(`
SELECT id, path, name, unix_timestamp(timestamp), size, is_dir
FROM filestore_metadata
WHERE path_hash = ? AND path = ? AND name = ?`, hash(dir_name),
		dir_name, base_name)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	for rows.Next() {
		row := &MysqlFileStoreFileInfo{}
		err = rows.Scan(&row.id, &row.path, &row.name,
			&row.timestamp, &row.size, &row.is_dir)
		if err != nil {
			return nil, err
		}

		// Only return the first row
		if true {
			return row, nil
		}
	}

	return nil, os.ErrNotExist
}

func (self *SqlFileStore) ListDirectory(dirname string) ([]os.FileInfo, error) {
	result := []os.FileInfo{}
	components := utils.SplitComponents(dirname)
	dir_name := utils.JoinComponents(components, "/")

	rows, err := db.Query(`
SELECT id, path, name, unix_timestamp(timestamp), size, is_dir
FROM filestore_metadata
WHERE path_hash = ? AND path = ?`, hash(dir_name), dir_name)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	for rows.Next() {
		row := &MysqlFileStoreFileInfo{}
		err = rows.Scan(&row.id, &row.path, &row.name,
			&row.timestamp, &row.size, &row.is_dir)
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

func (self *SqlFileStore) Delete(filename string) (err error) {
	components := utils.SplitComponents(filename)
	if len(components) > 0 {
		dir_path := utils.JoinComponents(components[:len(components)-1], "/")
		name := components[len(components)-1]

		ctx := context.Background()
		tx, err := db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelSerializable})
		if err != nil {
			return err
		}
		// Commit or rollback depending on error.
		defer func() {
			if err != nil {
				// Keep the original error
				_ = tx.Rollback()
				return
			}
			err = tx.Commit()
		}()

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
	}

	return nil
}

func (self *SqlFileStore) Get(filename string) ([]byte, bool) {
	return nil, false
}

func (self *SqlFileStore) Clear() {}

func NewSqlFileStore(config_obj *config_proto.Config) (api.FileStore, error) {
	my_sql_mu.Lock()
	defer my_sql_mu.Unlock()

	if db == nil {
		var err error
		db, err = initializeDatabase(
			config_obj, config_obj.Datastore.MysqlConnectionString,
			config_obj.Datastore.MysqlDatabase)
		if err != nil {
			return nil, err
		}
	}

	return &SqlFileStore{config_obj: config_obj}, nil
}

func initializeDatabase(
	config_obj *config_proto.Config,
	database_connection_string, database string) (*sql.DB, error) {

	db, err := sql.Open("mysql", database_connection_string)
	if err != nil {
		return nil, err
	}
	// Do not close the handle as it is a global.

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
			return nil, err
		}
		defer db.Close()

		_, err = db.Exec(fmt.Sprintf("create database if not exists `%v`",
			database))
		if err != nil {
			return nil, err
		}
	}

	_, err = db.Exec(`create table if not exists
    filestore(id int NOT NULL,
              part int NOT NULL DEFAULT 0,
              start_offset int,
              end_offset int,
              channel varchar(256),
              part_id INT NOT NULL AUTO_INCREMENT,
              data blob,
              PRIMARY KEY (part_id),
              unique INDEX(id, part, start_offset, end_offset),
              INDEX(channel, part_id),
              INDEX(id, start_offset))`)
	if err != nil {
		return nil, err
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
		return nil, err
	}

	return db, nil
}

func NewSqlFileStoreAccessor(file_store *SqlFileStore) *SqlFileStoreAccessor {
	return &SqlFileStoreAccessor{file_store}
}

type SqlFileStoreAccessor struct {
	file_store *SqlFileStore
}

func (self SqlFileStoreAccessor) New(scope *vfilter.Scope) glob.FileSystemAccessor {
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
	return &api.FileReaderAdapter{FileReader: fd}, nil
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
