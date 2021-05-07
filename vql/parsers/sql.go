package parsers

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"strings"
	"time"

	"github.com/Velocidex/ordereddict"
	_ "github.com/go-sql-driver/mysql"
	"github.com/jmoiron/sqlx"
	_ "github.com/lib/pq"
	utils "www.velocidex.com/golang/velociraptor/utils"
	vql_subsystem "www.velocidex.com/golang/velociraptor/vql"
	vfilter "www.velocidex.com/golang/vfilter"
	"www.velocidex.com/golang/vfilter/arg_parser"
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
	if connstring == "" {
		return nil, fmt.Errorf("file parameter required for %s driver!", driver)
	}

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

		// Make sure to close the connection when the query unwinds.
		err = vql_subsystem.GetRootScope(scope).AddDestructor(func() {
			client.Close()
		})
		if err != nil {
			client.Close()
			return nil, err
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
		err := arg_parser.ExtractArgsWithContext(ctx, scope, args, arg)
		if err != nil {
			scope.Log("sql: %v", err)
			return
		}

		if arg.Accessor == "" {
			arg.Accessor = "file"
		}

		var handle *sqlx.DB

		switch arg.Driver {
		default:
			scope.Log("sql: Unsupported driver %s!", arg.Driver)
			return

		case "sqlite":
			err = vql_subsystem.CheckFilesystemAccess(scope, arg.Accessor)
			if err != nil {
				scope.Log("sql: %s", err)
				return
			}

			handle, err = GetHandleSqlite(ctx, arg, scope)
			if err != nil {
				scope.Log("sql: %s", err)
				return
			}

		case "mysql", "postgres":
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
