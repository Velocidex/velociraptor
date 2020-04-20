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
package csv

import (
	"context"
	"io"
	"os"

	"github.com/Velocidex/ordereddict"
	"www.velocidex.com/golang/velociraptor/acls"
	"www.velocidex.com/golang/velociraptor/artifacts"
	"www.velocidex.com/golang/velociraptor/file_store"
	"www.velocidex.com/golang/velociraptor/file_store/csv"
	"www.velocidex.com/golang/velociraptor/glob"
	vql_subsystem "www.velocidex.com/golang/velociraptor/vql"
	vfilter "www.velocidex.com/golang/vfilter"
)

type ParseCSVPluginArgs struct {
	Filenames []string `vfilter:"required,field=filename,doc=CSV files to open"`
	Accessor  string   `vfilter:"optional,field=accessor,doc=The accessor to use"`
}

type ParseCSVPlugin struct{}

func (self ParseCSVPlugin) Call(
	ctx context.Context,
	scope *vfilter.Scope,
	args *ordereddict.Dict) <-chan vfilter.Row {
	output_chan := make(chan vfilter.Row)

	go func() {
		defer close(output_chan)

		arg := &ParseCSVPluginArgs{}
		err := vfilter.ExtractArgs(scope, args, arg)
		if err != nil {
			scope.Log("parse_csv: %s", err.Error())
			return
		}

		err = vql_subsystem.CheckFilesystemAccess(scope, arg.Accessor)
		if err != nil {
			scope.Log("parse_csv: %s", err)
			return
		}

		for _, filename := range arg.Filenames {
			func() {
				accessor, err := glob.GetAccessor(arg.Accessor, scope)
				if err != nil {
					scope.Log("parse_csv: %v", err)
					return
				}
				fd, err := accessor.Open(filename)
				if err != nil {
					scope.Log("Unable to open file %s: %v",
						filename, err)
					return
				}
				defer fd.Close()

				csv_reader := csv.NewReader(fd)
				headers, err := csv_reader.Read()
				if err != nil {
					return
				}

				for {
					row := ordereddict.NewDict()
					row_data, err := csv_reader.ReadAny()
					if err != nil {
						if err != io.EOF {
							scope.Log("parse_csv: %v", err)
						}
						return
					}

					for idx, row_item := range row_data {
						if idx > len(headers) {
							break
						}
						row.Set(headers[idx], row_item)
					}

					output_chan <- row
				}
			}()
		}
	}()

	return output_chan
}

func (self ParseCSVPlugin) Info(scope *vfilter.Scope, type_map *vfilter.TypeMap) *vfilter.PluginInfo {
	return &vfilter.PluginInfo{
		Name:    "parse_csv",
		Doc:     "Parses events from a CSV file.",
		ArgType: type_map.AddType(scope, &ParseCSVPluginArgs{}),
	}
}

type _WatchCSVPlugin struct{}

func (self _WatchCSVPlugin) Call(
	ctx context.Context,
	scope *vfilter.Scope,
	args *ordereddict.Dict) <-chan vfilter.Row {
	output_chan := make(chan vfilter.Row)

	go func() {
		// Do not close output_chan - The event log service
		// owns it and it will be closed by it.

		arg := &ParseCSVPluginArgs{}
		err := vfilter.ExtractArgs(scope, args, arg)
		if err != nil {
			scope.Log("watch_csv: %s", err.Error())
			return
		}

		err = vql_subsystem.CheckFilesystemAccess(scope, arg.Accessor)
		if err != nil {
			scope.Log("watch_csv: %s", err)
			return
		}

		// Register the output channel as a listener to the
		// global event.
		for _, filename := range arg.Filenames {
			GlobalCSVService.Register(
				filename, arg.Accessor,
				ctx, scope, output_chan)
		}

		// Wait until the query is complete.
		<-ctx.Done()

	}()

	return output_chan
}

func (self _WatchCSVPlugin) Info(scope *vfilter.Scope, type_map *vfilter.TypeMap) *vfilter.PluginInfo {
	return &vfilter.PluginInfo{
		Name: "watch_csv",
		Doc: "Watch a CSV file and stream events from it. " +
			"Note: This is an event plugin which does not complete.",
		ArgType: type_map.AddType(scope, &ParseCSVPluginArgs{}),
	}
}

type WriteCSVPluginArgs struct {
	Filename string              `vfilter:"required,field=filename,doc=CSV files to open"`
	Accessor string              `vfilter:"optional,field=accessor,doc=The accessor to use"`
	Query    vfilter.StoredQuery `vfilter:"required,field=query,doc=query to write into the file."`
}

type WriteCSVPlugin struct{}

func (self WriteCSVPlugin) Call(
	ctx context.Context,
	scope *vfilter.Scope,
	args *ordereddict.Dict) <-chan vfilter.Row {
	output_chan := make(chan vfilter.Row)

	go func() {
		defer close(output_chan)

		arg := &WriteCSVPluginArgs{}
		err := vfilter.ExtractArgs(scope, args, arg)
		if err != nil {
			scope.Log("write_csv: %s", err.Error())
			return
		}

		var writer *csv.CSVWriter

		switch arg.Accessor {
		case "", "file":
			err := vql_subsystem.CheckAccess(scope, acls.FILESYSTEM_WRITE)
			if err != nil {
				scope.Log("write_csv: %s", err)
				return
			}

			file, err := os.OpenFile(arg.Filename, os.O_RDWR|os.O_CREATE, 0700)
			if err != nil {
				scope.Log("write_csv: Unable to open file %s: %s",
					arg.Filename, err.Error())
				return
			}
			defer file.Close()

			writer = csv.GetCSVAppender(scope, file, true)
			defer writer.Close()

		case "fs":
			err := vql_subsystem.CheckAccess(scope, acls.SERVER_ADMIN)
			if err != nil {
				scope.Log("write_csv: %s", err)
				return
			}

			config_obj, ok := artifacts.GetServerConfig(scope)
			if !ok {
				scope.Log("Command can only run on the server")
				return
			}

			file_store_factory := file_store.GetFileStore(config_obj)
			file, err := file_store_factory.WriteFile(arg.Filename)
			if err != nil {
				scope.Log("write_csv: Unable to open file %s: %v",
					arg.Filename, err)
				return
			}
			defer file.Close()

			file.Truncate()

			writer, err = csv.GetCSVWriter(scope, file)
			if err != nil {
				scope.Log("write_csv: Unable to open file %s: %v",
					arg.Filename, err)
				return
			}

			defer writer.Close()

		default:
			scope.Log("write_csv: Unsupported accessor for writing %v", arg.Accessor)
			return
		}

		for row := range arg.Query.Eval(ctx, scope) {
			writer.Write(row)
			output_chan <- row
		}
	}()

	return output_chan
}

func (self WriteCSVPlugin) Info(scope *vfilter.Scope, type_map *vfilter.TypeMap) *vfilter.PluginInfo {
	return &vfilter.PluginInfo{
		Name:    "write_csv",
		Doc:     "Write a query into a CSV file.",
		ArgType: type_map.AddType(scope, &WriteCSVPluginArgs{}),
	}
}

func init() {
	vql_subsystem.RegisterPlugin(&ParseCSVPlugin{})
	vql_subsystem.RegisterPlugin(&_WatchCSVPlugin{})
	vql_subsystem.RegisterPlugin(&WriteCSVPlugin{})
}
