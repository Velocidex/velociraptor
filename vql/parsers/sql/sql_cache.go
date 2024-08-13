package sql

import (
	"context"
	"errors"
	"io/ioutil"
	"os"
	"strings"
	"sync"
	"time"

	_ "github.com/mattn/go-sqlite3"

	"database/sql"

	"www.velocidex.com/golang/velociraptor/accessors"
	"www.velocidex.com/golang/velociraptor/constants"
	utils "www.velocidex.com/golang/velociraptor/utils"
	vql_subsystem "www.velocidex.com/golang/velociraptor/vql"
	vfilter "www.velocidex.com/golang/vfilter"
	"www.velocidex.com/golang/vfilter/types"
)

const (
	SQL_CACHE_TAG = "$SQL_CACHE_TAG"
)

type DB struct {
	*sql.DB
	tmpfile string
	scope   types.Scope

	mu       sync.Mutex
	in_use   int
	last_use int64
}

func (self *DB) ShouldExpire(now int64) bool {
	self.mu.Lock()
	defer self.mu.Unlock()

	return self.in_use <= 0 &&
		now-self.last_use > 5
}

func (self *DB) SetInUse() {
	self.mu.Lock()
	defer self.mu.Unlock()

	self.in_use++
}

func (self *DB) remove() {
	// Try to remove it immediately
	err := os.Remove(self.tmpfile)
	if err == nil || errors.Is(err, os.ErrNotExist) {
		self.scope.Log("DEBUG:Removed tempfile: %v", self.tmpfile)
		return
	}

	// On windows especially, we can not remove files that are opened
	// by something else, so we keep trying for a while.
	for i := 0; i < 100; i++ {
		err := os.Remove(self.tmpfile)
		if err == nil || errors.Is(err, os.ErrNotExist) {
			self.scope.Log("DEBUG:Removed tempfile: %v after %v tries",
				self.tmpfile, i)
			return
		}
		time.Sleep(time.Second)
	}
	self.scope.Log("ERROR:Error removing file: %v", err)
}

func (self *DB) Close() {
	self.mu.Lock()
	defer self.mu.Unlock()

	self.in_use--
	self.last_use = utils.GetTime().Now().Unix()
}

func (self *DB) Destroy() {
	self.mu.Lock()
	defer self.mu.Unlock()

	err := self.DB.Close()
	if err != nil {
		self.scope.Log("ERROR:Handle %v can not close: %v\n",
			self.tmpfile, err)
	}
	if self.tmpfile != "" {
		self.remove()
		self.tmpfile = ""
	}
}

type sqlCache struct {
	cache map[string]*DB
	scope types.Scope

	mu     sync.Mutex
	closed bool
}

func (self *sqlCache) Reap() {
	self.mu.Lock()
	defer self.mu.Unlock()

	self.reap()
}

func (self *sqlCache) reap() {
	now := utils.GetTime().Now().Unix()

	for k, v := range self.cache {
		if v.ShouldExpire(now) {
			v.Destroy()
			delete(self.cache, k)
		}
	}
}

func (self *sqlCache) Close() {
	self.mu.Lock()
	defer self.mu.Unlock()

	self.closed = true

	for k, handle := range self.cache {
		handle.Destroy()
		delete(self.cache, k)
	}
}

func (self *sqlCache) GetHandleSqlite(ctx context.Context,
	arg *SQLPluginArgs, scope vfilter.Scope) (handle *DB, err error) {
	var sql_handle *sql.DB

	self.mu.Lock()
	defer self.mu.Unlock()

	if self.closed {
		return nil, errors.New("Cache closed")
	}

	accessor, err := accessors.GetAccessor(arg.Accessor, scope)
	if err != nil {
		return nil, err
	}

	if arg.Filename == nil {
		return nil, errors.New("file parameter required for sqlite driver!")
	}
	filename := arg.Filename.String()
	if filename == "" {
		return nil, errors.New("file parameter required for sqlite driver!")
	}

	// cache key.
	key := filename + arg.Accessor
	handle, ok := self.cache[key]
	if ok {
		handle.SetInUse()
		return handle, nil
	}

	// Check the header quickly to ensure that we dont copy the
	// file needlessly. If the file does not exist, we allow a
	// connection because this will create a new file.
	header_ok, err := checkSQLiteHeader(scope, accessor, arg.Filename)
	if !errors.Is(err, os.ErrNotExist) && !header_ok {
		return nil, notValidDatabase
	}

	// If needed we save the file to a tempfile
	tempfile := ""
	should_make_copy := vql_subsystem.GetBoolFromRow(
		scope, scope, constants.SQLITE_ALWAYS_MAKE_TEMPFILE)

	if !should_make_copy {
		// We need raw file access to use the sqlite library directly.
		raw_accessor, ok := accessor.(accessors.RawFileAPIAccessor)
		if !ok {
			should_make_copy = true
		} else {
			filename, err = raw_accessor.GetUnderlyingAPIFilename(arg.Filename)
			if err != nil {
				should_make_copy = true
			}
		}
	}

	// When using the file accessor it is possible to pass sqlite
	// options by encoding them into the filename. In this case we
	// need to extract the filename (from before the ?) so we can
	// copy it over.
	parts := strings.SplitN(filename, "?", 2)

	// We need to operate on a copy.
	if should_make_copy {
		tempfile, err = _MakeTempfile(ctx, arg, filename, scope)
		if err != nil {
			return nil, err
		}

		filename_with_options := tempfile
		if len(parts) > 1 {
			filename_with_options += "?" + parts[1]
		}

		// If we failed to open the copy, we dont make another copy -
		// just fail!
		sql_handle, err = sql.Open("sqlite3", filename_with_options)
		if err == nil {
			err = sql_handle.Ping()
		}
		if err != nil {
			scope.Log("ERROR:Unable to open sqlite file %v: %v", tempfile, err)
			os.Remove(tempfile)
			return nil, err
		}

	} else {
		// Open the original file inline. Note: filename may have
		// options
		sql_handle, err = sql.Open("sqlite3", filename)
		if err == nil {
			err = sql_handle.Ping()
		}
		if err != nil {
			// An error occurred maybe the database is locked, we try to
			// copy it to temp file and try again.
			if arg.Accessor != "data" {
				scope.Log("ERROR:Unable to open sqlite file %v: %v", filename, err)
			} else {
				scope.Log("ERROR:Unable to open sqlite file: %v", err)
			}

			// If the database is missing etc we just return the error,
			// but locked files are handled especially.
			if !strings.Contains(err.Error(), "locked") {
				return nil, err
			}

			var err1 error
			tempfile, err1 = _MakeTempfile(ctx, arg, parts[0], scope)
			if err1 != nil {
				scope.Log("ERROR:sqlite: Unable to create temp file: %v", err1)
				return nil, err1
			}

			scope.Log("DEBUG:Sqlite file %v is locked with %v, creating a local copy on %v",
				filename, err, tempfile)

			filename_with_options := tempfile
			if len(parts) > 1 {
				filename_with_options += "?" + parts[1]
			}

			sql_handle, err = sql.Open("sqlite3", filename_with_options)
			if err == nil {
				err = sql_handle.Ping()
			}
			if err != nil {
				scope.Log("ERROR:Unable to open sqlite file %v: %v", tempfile, err)
				os.Remove(tempfile)
				return nil, err
			}
		}
	}

	// If we get here, sqlx_handle is valid - wrap it in the cache and return it.
	result := &DB{
		DB:      sql_handle,
		scope:   scope,
		tmpfile: tempfile, // This will be empty if we didnt use a temp file.
		in_use:  1,        // We have one user - our caller.
	}
	self.cache[key] = result

	self.reap()

	return result, nil
}

func NewSQLCache(ctx context.Context, scope types.Scope) *sqlCache {
	result := &sqlCache{
		cache: make(map[string]*DB),
		scope: scope,
	}

	// Close the entire cache when the scope is done.
	vql_subsystem.GetRootScope(scope).AddDestructor(result.Close)

	go func() {
		select {
		case <-ctx.Done():
			return

		case <-utils.GetTime().After(time.Second):
			result.Reap()
		}
	}()

	return result
}

// Check the file header - ignore if this is not really an sqlite
// file.
func checkSQLiteHeader(scope vfilter.Scope,
	accessor accessors.FileSystemAccessor,
	filename *accessors.OSPath) (bool, error) {

	parts := strings.SplitN(filename.Basename(), "?", 2)
	filename_without_options := filename.Dirname().Append(parts[0])

	file, err := accessor.OpenWithOSPath(filename_without_options)
	if err != nil {
		return false, err
	}
	defer file.Close()

	header := make([]byte, 12)
	_, err = file.Read(header)
	if err != nil {
		return false, err
	}

	return string(header) == "SQLite forma", nil
}

func GetHandleSqlite(ctx context.Context,
	arg *SQLPluginArgs, scope vfilter.Scope) (handle *DB, err error) {

	sql_cache, ok := vql_subsystem.CacheGet(scope, SQL_CACHE_TAG).(*sqlCache)
	if !ok {
		sql_cache = NewSQLCache(ctx, scope)
	}

	vql_subsystem.CacheSet(scope, SQL_CACHE_TAG, sql_cache)

	return sql_cache.GetHandleSqlite(ctx, arg, scope)
}

func _MakeTempfile(ctx context.Context,
	arg *SQLPluginArgs, filename string,
	scope vfilter.Scope) (
	string, error) {

	tmpfile, err := ioutil.TempFile("", "tmp*.sqlite")
	if err != nil {
		return "", err
	}
	defer tmpfile.Close()

	fs, err := accessors.GetAccessor(arg.Accessor, scope)
	if err != nil {
		return "", err
	}

	file, err := fs.Open(filename)
	if err != nil {
		return "", err
	}
	defer file.Close()

	_, err = utils.Copy(ctx, tmpfile, file)
	if err != nil {
		return "", err
	}

	return tmpfile.Name(), nil
}
