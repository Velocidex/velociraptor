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
	"time"

	"www.velocidex.com/golang/velociraptor/file_store/csv"
	"www.velocidex.com/golang/velociraptor/glob"
	vql_subsystem "www.velocidex.com/golang/velociraptor/vql"
	vfilter "www.velocidex.com/golang/vfilter"
)

type ParseCSVPluginArgs struct {
	Filenames []string `vfilter:"required,field=filename"`
	Accessor  string   `vfilter:"optional,field=accessor"`
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
				accessor := glob.GetAccessor(arg.Accessor, ctx)
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

		accessor := glob.GetAccessor(arg.Accessor, ctx)

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
						break
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
							break
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

func init() {
	vql_subsystem.RegisterPlugin(&ParseCSVPlugin{})
	vql_subsystem.RegisterPlugin(&_WatchCSVPlugin{})
}
