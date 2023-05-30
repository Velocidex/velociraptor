/*
   Velociraptor - Dig Deeper
   Copyright (C) 2019-2022 Rapid7 Inc.

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

// This plugin provides support for parsing sqlite files. Because we
// use the actual library we must provide it with a file on
// disk. Since VQL may specify an arbitrary accessor, we can make a
// temp copy of the sqlite file in order to query it. The temp copy
// remains alive for the duration of the query, and we will cache it.
package parsers

import (
	"context"
	"errors"
	"io/ioutil"
	"os"
	"strings"
	"time"

	"github.com/Velocidex/ordereddict"
	"github.com/jmoiron/sqlx"
	_ "github.com/mattn/go-sqlite3"
	"www.velocidex.com/golang/velociraptor/accessors"
	"www.velocidex.com/golang/velociraptor/constants"
	utils "www.velocidex.com/golang/velociraptor/utils"
	vql_subsystem "www.velocidex.com/golang/velociraptor/vql"
	vfilter "www.velocidex.com/golang/vfilter"
)

type SQLiteArgs struct {
	Filename string      `vfilter:"required,field=file"`
	Accessor string      `vfilter:"optional,field=accessor,doc=The accessor to use."`
	Query    string      `vfilter:"required,field=query"`
	Args     vfilter.Any `vfilter:"optional,field=args"`
}

type SQLitePlugin struct{}

func (self SQLitePlugin) Call(
	ctx context.Context,
	scope vfilter.Scope,
	args *ordereddict.Dict) <-chan vfilter.Row {

	args.Set("driver", "sqlite")

	// This is just an alias for the sql plugin.
	return SQLPlugin{}.Call(ctx, scope, args)
}

// Velociraptor always uses the path separator at the root of
// filesystem (on windows this means before the drive letter). This
// convension confuses the sqlite driver. So convert
// "\C:\Windows\X.sqlite" to "C:\Windows\X.sqlite"
func VFSPathToFilesystemPath(path string) string {
	return strings.TrimPrefix(path, "\\")
}

// Check the file header - ignore if this is not really an sqlite
// file.
func checkSQLiteHeader(scope vfilter.Scope, accessor, filename string) (bool, error) {
	fs, err := accessors.GetAccessor(accessor, scope)
	if err != nil {
		return false, err
	}

	file, err := fs.Open(filename)
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
	arg *SQLPluginArgs, scope vfilter.Scope) (
	handle *sqlx.DB, err error) {
	filename := VFSPathToFilesystemPath(arg.Filename)

	if filename == "" {
		return nil, errors.New("file parameter required for sqlite driver!")
	}

	key := "sqlite_" + filename + arg.Accessor
	handle, ok := vql_subsystem.CacheGet(scope, key).(*sqlx.DB)
	if !ok {
		// Check the header quickly to ensure that we dont copy the
		// file needlessly. If the file does not exist, we allow a
		// connection because this will create a new file.
		header_ok, err := checkSQLiteHeader(scope, arg.Accessor, filename)
		if !errors.Is(err, os.ErrNotExist) && !header_ok {
			return nil, notValidDatabase
		}

		should_make_copy := vql_subsystem.GetBoolFromRow(scope, scope, constants.SQLITE_ALWAYS_MAKE_TEMPFILE)
		if arg.Accessor != "file" && arg.Accessor != "" {
			should_make_copy = true
		}

		if !should_make_copy {
			handle, err = sqlx.Connect("sqlite3", filename)
			if err != nil {
				// An error occurred maybe the database
				// is locked, we try to copy it to
				// temp file and try again.
				if arg.Accessor != "data" {
					scope.Log("Unable to open sqlite file %v: %v",
						filename, err)
				} else {
					scope.Log("Unable to open sqlite file: %v", err)
				}

				// If the database is missing etc we just return the
				// error, but locked files are handled especially.
				if !strings.Contains(err.Error(), "locked") {
					return nil, err
				}

				scope.Log("Sqlite file %v is locked with %v, creating a local copy",
					filename, err)

				// When using the file accessor it is possible to pass
				// sqlite options by encoding them into the
				// filename. In this case we need to extract the
				// filename (from before the ?) so we can copy it
				// over.
				parts := strings.Split(filename, "?")
				filename, err = _MakeTempfile(ctx, arg, parts[0], scope)
				if err != nil {
					scope.Log("Unable to create temp file: %v", err)
					return nil, err
				}
				scope.Log("Using local copy %v", filename)
			}

			// All other accessors, make a copy and
			// operate on that.
		} else {
			filename, err = _MakeTempfile(ctx, arg, filename, scope)
			if err != nil {
				return nil, err
			}
			scope.Log("Using local copy %v", filename)
		}

		// Try once again to connect to the new file
		if handle == nil {
			handle, err = sqlx.Connect("sqlite3", filename)
			if err != nil {
				return nil, err
			}
		}

		vql_subsystem.CacheSet(scope, key, handle)

		// Add the destructor to the root scope to ensure we
		// dont get closed too early.
		err = vql_subsystem.GetRootScope(scope).AddDestructor(func() {
			handle.Close()
		})
		if err != nil {
			scope.Log("Unable to set destructor for %v", filename)
			handle.Close()
			return nil, err
		}
	}
	return handle, nil
}

func _MakeTempfile(ctx context.Context,
	arg *SQLPluginArgs, filename string,
	scope vfilter.Scope) (
	string, error) {

	if arg.Accessor != "data" {
		scope.Log("Will try to copy %v to temp file", filename)
	}

	tmpfile, err := ioutil.TempFile("", "tmp*.sqlite")
	if err != nil {
		return "", err
	}
	defer tmpfile.Close()

	// Make sure the file is removed when the query is done.
	remove := func() {
		// Try to remove it immediately
		err := os.Remove(tmpfile.Name())
		if err == nil || errors.Is(err, os.ErrNotExist) {
			scope.Log("sqlite: removing tempfile %v", tmpfile.Name())
			return
		}

		// On windows especially we can not remove files that
		// are opened by something else, so we keep trying for
		// a while.
		go func() {
			for i := 0; i < 100; i++ {
				err := os.Remove(tmpfile.Name())
				if err == nil || errors.Is(err, os.ErrNotExist) {
					scope.Log("sqlite: removing tempfile %v", tmpfile.Name())
					return
				}
				time.Sleep(time.Second)
			}
			scope.Log("Error removing file: %v", err)
		}()
	}
	err = scope.AddDestructor(remove)
	if err != nil {
		go remove()
		return "", err
	}

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

func (self SQLitePlugin) Info(scope vfilter.Scope, type_map *vfilter.TypeMap) *vfilter.PluginInfo {
	return &vfilter.PluginInfo{
		Name:    "sqlite",
		Doc:     "Opens an SQLite file and run a query against it (This is an alias to the sql() plugin which supports more database types).",
		ArgType: type_map.AddType(scope, &SQLiteArgs{}),
	}
}

func init() {
	vql_subsystem.RegisterPlugin(&SQLitePlugin{})
}
