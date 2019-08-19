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
package parsers

import (
	"context"
	"io"
	"os"
	"time"

	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
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
	args *vfilter.Dict) <-chan vfilter.Row {
	output_chan := make(chan vfilter.Row)

	go func() {
		defer close(output_chan)

		arg := &ParseCSVPluginArgs{}
		err := vfilter.ExtractArgs(scope, args, arg)
		if err != nil {
			scope.Log("parse_csv: %s", err.Error())
			return
		}

		for _, filename := range arg.Filenames {
			func() {
				accessor, err := glob.GetAccessor(arg.Accessor, ctx)
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
					row := vfilter.NewDict()
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
	args *vfilter.Dict) <-chan vfilter.Row {
	output_chan := make(chan vfilter.Row)

	go func() {
		defer close(output_chan)

		arg := &ParseCSVPluginArgs{}
		err := vfilter.ExtractArgs(scope, args, arg)
		if err != nil {
			scope.Log("watch_evtx: %s", err.Error())
			return
		}

		accessor, err := glob.GetAccessor(arg.Accessor, ctx)
		if err != nil {
			scope.Log("watch_evtx: %v", err)
			return
		}

		// A map between file name and the last offset we read.
		last_offset_map := make(map[string]int64)

		// Parse the files once to get the last event
		// id. After this we will watch for new events added
		// to the file.
		for _, filename := range arg.Filenames {
			func() {
				fd, err := accessor.Open(filename)
				if err != nil {
					return
				}
				defer fd.Close()

				// Skip all the rows until the end.
				csv_reader := csv.NewReader(fd)
				for {
					_, err := csv_reader.ReadAny()
					if err != nil {
						return
					}
				}

				last_offset_map[filename] = csv_reader.ByteOffset
			}()
		}

		for {
			for _, filename := range arg.Filenames {
				func() {
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

					last_offset := last_offset_map[filename]

					// Seek to the last place we were.
					fd.Seek(last_offset, 0)

					for {
						row_data, err := csv_reader.ReadAny()
						if err != nil {
							return
						}

						row := vfilter.NewDict()
						for idx, row_item := range row_data {
							if idx > len(headers) {
								break
							}
							row.Set(headers[idx], row_item)
						}

						output_chan <- row
					}

					last_offset_map[filename] = csv_reader.ByteOffset
				}()
			}

			time.Sleep(10 * time.Second)
		}
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
	Query    vfilter.StoredQuery `vfilter:"required,field=query,doc=query to write into the file."`
}

type WriteCSVPlugin struct{}

func (self WriteCSVPlugin) Call(
	ctx context.Context,
	scope *vfilter.Scope,
	args *vfilter.Dict) <-chan vfilter.Row {
	output_chan := make(chan vfilter.Row)

	go func() {
		defer close(output_chan)

		// Check the config if we are allowed to execve at all.
		scope_config, pres := scope.Resolve("config")
		if pres {
			config_obj, ok := scope_config.(*config_proto.ClientConfig)
			if ok && config_obj.PreventExecve {
				scope.Log("write_csv: Not allowed to write files by configuration.")
				return
			}
		}

		arg := &WriteCSVPluginArgs{}
		err := vfilter.ExtractArgs(scope, args, arg)
		if err != nil {
			scope.Log("write_csv: %s", err.Error())
			return
		}

		file, err := os.OpenFile(arg.Filename, os.O_RDWR|os.O_CREATE, 0700)
		if err != nil {
			scope.Log("write_csv: Unable to open file %s: %s",
				arg.Filename, err.Error())
			return
		}
		defer file.Close()

		writer := csv.NewWriter(file)
		defer writer.Flush()

		columns := []string{}
		for row := range arg.Query.Eval(ctx, scope) {
			if len(columns) == 0 {
				columns = scope.GetMembers(row)
				if len(columns) > 0 {
					file.Truncate(0)
					err := writer.Write(columns)
					if err != nil {
						scope.Log("write_csv: %s", err.Error())
						return
					}
				}
			}

			new_row := []interface{}{}
			for _, column := range columns {
				item, pres := scope.Associative(row, column)
				if !pres {
					item = vfilter.Null{}
				}

				new_row = append(new_row, item)
			}

			err := writer.WriteAny(new_row)
			if err != nil {
				scope.Log("write_csv: %s", err.Error())
				return
			}

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
