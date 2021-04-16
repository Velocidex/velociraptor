//+build extras
package parsers

import (
	"context"
	"errors"
	"fmt"
	"io/ioutil"
	"os"
	"reflect"
	"strings"
	"time"

	"github.com/Velocidex/ordereddict"
	"github.com/jmoiron/sqlx"
	_ "github.com/lib/pq"
	_ "github.com/mattn/go-sqlite3"
	_ "github.com/go-sql-driver/mysql"
	"www.velocidex.com/golang/velociraptor/glob"
	utils "www.velocidex.com/golang/velociraptor/utils"
	vql_subsystem "www.velocidex.com/golang/velociraptor/vql"
	vfilter "www.velocidex.com/golang/vfilter"
)

type SQLPluginArgs struct {
	Driver     string      `vfilter:"required,field=driver, doc=sqlite, mysql,or postgres"`
	ConnString string      `vfilter:"optional,field=connstring, doc=SQL Connection String"`
	Filename   string      `vfilter:"optional,field=file, doc=Required if using sqlite driver"`
	Accessor   string      `vfilter:"optional,field=accessor,doc=The accessor to use if using sqlite"`
	Query      string      `vfilter:"required,field=query"`
	Args       vfilter.Any `vfilter:"optional,field=args"`
}

type SQLPlugin struct{}

// Get DB handle from cache if it exists, else create a new connection
func (self SQLPlugin) GetHandleOther(scope vfilter.Scope, connstring string, driver string) (*sqlx.DB, error) {
	cacheKey := fmt.Sprintf("%s %s", driver, connstring)
	client := vql_subsystem.CacheGet(scope, cacheKey)

	if client == nil {
		client, err := sqlx.Open(driver, connstring)
		if err != nil {
			return nil, err
		}
		if driver == "mysql" {
			// Important settings according to mysql driver README
			client.SetConnMaxLifetime(time.Minute * 3)
			client.SetMaxOpenConns(10)
			client.SetMaxIdleConns(10)
		}
		return client, nil

	}
	switch t := client.(type) {
	case error:
		return nil, t
	case *sqlx.DB:
		return t, nil
	default:
		return nil, errors.New("Error")
	}

}

func (self SQLPlugin) Call(
	ctx context.Context,
	scope vfilter.Scope,
	args *ordereddict.Dict) <-chan vfilter.Row {
	output_chan := make(chan vfilter.Row)
	go func() {
		defer close(output_chan)
		defer utils.RecoverVQL(scope)

		arg := &SQLPluginArgs{}
		err := vfilter.ExtractArgs(scope, args, arg)
		if err != nil {
			scope.Log("sql: %v", err)
			return
		}

		if arg.Accessor == "" {
			arg.Accessor = "file"
		}

		if arg.Driver != "sqlite" && arg.Driver != "mysql" && arg.Driver != "postgres" {
			scope.Log("sql: Unsupported driver %s!", arg.Driver)
			return
		}

		var handle *sqlx.DB
		if arg.Driver == "sqlite" {
			if arg.Filename == "" {
				scope.Log("sql: file parameter required for sqlite driver!")
				return
			}
			handle, err = self.GetHandleSqlite(ctx, arg, scope)
			if err != nil {
				scope.Log("sql: %s", err)
				return
			}
			err = vql_subsystem.CheckFilesystemAccess(scope, arg.Accessor)
			if err != nil {
				scope.Log("sql: %s", err)
				return
			}
		} else {
			if arg.ConnString == "" {
				scope.Log("sql: file parameter required for %s driver!", arg.Driver)
				return
			}
			handle, err = self.GetHandleOther(scope, arg.ConnString, arg.Driver)
			if err != nil {
				scope.Log("sql: %s", err)
				return
			}
		}

		query_parameters := []interface{}{}
		if arg.Args != nil {
			args_value := reflect.Indirect(reflect.ValueOf(arg.Args))
			if args_value.Type().Kind() != reflect.Slice {
				query_parameters = append(query_parameters, arg.Args)
			} else {
				for i := 0; i < args_value.Len(); i++ {
					query_parameters = append(query_parameters,
						args_value.Index(i).Interface())
				}
			}
		}

		query := strings.TrimSpace(arg.Query)
		if query == "" {
			return
		}
		rows, err := handle.Queryx(query, query_parameters...)
		if err != nil {
			scope.Log("sql: %v", err)
			return
		}
		defer rows.Close()
		columns, err := rows.Columns()
		if err != nil {
			scope.Log("sql: %s", err)
		}
		for rows.Next() {
			row := ordereddict.NewDict()
			values, err := rows.SliceScan()
			if err != nil {
				scope.Log("sql: %v", err)
				return
			}

			for idx, item := range columns {
				var value interface{} = values[idx]
				bytes_value, ok := value.([]byte)
				if ok {
					value = string(bytes_value)
				}
				row.Set(item, value)
			}

			select {
			case <-ctx.Done():
				return

			case output_chan <- row:
			}
		}

	}()
	return output_chan
}

func VFSPathToFilesystemPath(path string) string {
	return strings.TrimPrefix(path, "\\")
}
// Get DB Handle for SQLite Databases
func (self SQLPlugin) GetHandleSqlite(
	ctx context.Context,
	arg *SQLPluginArgs, scope vfilter.Scope) (
	handle *sqlx.DB, err error) {
	filename := VFSPathToFilesystemPath(arg.Filename)

	key := "sqlite_" + filename + arg.Accessor
	handle, ok := vql_subsystem.CacheGet(scope, key).(*sqlx.DB)
	if !ok {
		if arg.Accessor == "file" {
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
				if !strings.Contains(err.Error(), "locked") {
					return nil, err
				}
				scope.Log("Sqlite file %v is locked with %v, creating a local copy",
					filename, err)
				filename, err = self._MakeTempfile(ctx, arg, filename, scope)
				if err != nil {
					scope.Log("Unable to create temp file: %v", err)
					return nil, err
				}
				scope.Log("Using local copy %v", filename)

			}
		} else {
			filename, err = self._MakeTempfile(ctx, arg, filename, scope)
			if err != nil {
				return nil, err
			}
		}
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
			handle.Close()
			return nil, err
		}
	}
	return handle, nil
}

func (self SQLPlugin) _MakeTempfile(
	ctx context.Context,
	arg *SQLPluginArgs, filename string,
	scope vfilter.Scope) (
	string, error) {

	if arg.Accessor != "data" {
		scope.Log("Will try to copy to temp file: %v", filename)
	}

	tmpfile, err := ioutil.TempFile("", "tmp*.sqlite")
	if err != nil {
		return "", err
	}
	defer tmpfile.Close()

	// Make sure the file is removed when the query is done.
	remove := func() {
		scope.Log("sql: removing tempfile %v", tmpfile.Name())
		err = os.Remove(tmpfile.Name())
		if err != nil {
			scope.Log("Error removing file: %v", err)
		}
	}
	err = scope.AddDestructor(remove)
	if err != nil {
		remove()
		return "", err
	}

	fs, err := glob.GetAccessor(arg.Accessor, scope)
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

func (self SQLPlugin) Info(scope vfilter.Scope, type_map *vfilter.TypeMap) *vfilter.PluginInfo {
	return &vfilter.PluginInfo{
		Name:    "sql",
		Doc:     "Run queries against sqlite, mysql, and postgres databases",
		ArgType: type_map.AddType(scope, &SQLPluginArgs{}),
	}
}

func init() {
	vql_subsystem.RegisterPlugin(&SQLPlugin{})
}
