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
	"fmt"
	"io"
	"os"

	"github.com/Velocidex/ordereddict"
	"www.velocidex.com/golang/velociraptor/acls"
	"www.velocidex.com/golang/velociraptor/file_store/csv"
	"www.velocidex.com/golang/velociraptor/glob"
	vql_subsystem "www.velocidex.com/golang/velociraptor/vql"
	vfilter "www.velocidex.com/golang/vfilter"
	"www.velocidex.com/golang/vfilter/arg_parser"
)

type ParseCSVPluginArgs struct {
	Filenames   []string `vfilter:"required,field=filename,doc=CSV files to open"`
	Accessor    string   `vfilter:"optional,field=accessor,doc=The accessor to use"`
	AutoHeaders bool     `vfilter:"optional,field=auto_headers,doc=If unset the first row is headers"`
	Separator   string   `vfilter:"optional,field=separator,doc=Comma separator (default ',')"`
	Comment     string   `vfilter:"optional,field=comment,doc=The single character that should be considered a comment"`
	Columns     []string `vfilter:"optional,field=columns,doc=The columns to use"`
}

type ParseCSVPlugin struct{}

func (self ParseCSVPlugin) Call(
	ctx context.Context,
	scope vfilter.Scope,
	args *ordereddict.Dict) <-chan vfilter.Row {
	output_chan := make(chan vfilter.Row)

	go func() {
		defer close(output_chan)

		arg := &ParseCSVPluginArgs{}
		err := arg_parser.ExtractArgsWithContext(ctx, scope, args, arg)
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
				csv_reader.TrimLeadingSpace = true

				if arg.Separator != "" {
					if len(arg.Separator) != 1 {
						scope.Log("parse_csv: separator can only be one character")
						return
					}
					csv_reader.Comma = rune(arg.Separator[0])
				}

				if arg.Comment != "" {
					if len(arg.Comment) != 1 {
						scope.Log("parse_csv: comment can only be one character")
						return
					}
					csv_reader.Comment = rune(arg.Comment[0])
				}

				if !arg.AutoHeaders && len(arg.Columns) == 0 {
					arg.Columns, err = csv_reader.Read()
					if err != nil {
						return
					}
				}

				var headers []string
				for {
					row := ordereddict.NewDict()
					row_data, err := csv_reader.ReadAny()
					if err == io.EOF {
						return
					}

					if err != nil {
						// Report the error and skip to the next record
						scope.Log("parse_csv: %v", err)
						continue
					}

					if headers == nil {
						headers = make([]string, 0, len(row_data))
						for idx := range row_data {
							if idx >= len(arg.Columns) {
								headers = append(headers,
									fmt.Sprintf("Col%v", idx))
							} else {
								headers = append(headers, arg.Columns[idx])
							}
						}
					}

					for idx, row_item := range row_data {
						if idx > len(headers) {
							break
						}
						row.Set(headers[idx], row_item)
					}

					select {
					case <-ctx.Done():
						return

					case output_chan <- row:
					}
				}
			}()
		}
	}()

	return output_chan
}

func (self ParseCSVPlugin) Info(scope vfilter.Scope, type_map *vfilter.TypeMap) *vfilter.PluginInfo {
	return &vfilter.PluginInfo{
		Name:    "parse_csv",
		Doc:     "Parses events from a CSV file.",
		ArgType: type_map.AddType(scope, &ParseCSVPluginArgs{}),
	}
}

type _WatchCSVPlugin struct{}

func (self _WatchCSVPlugin) Call(
	ctx context.Context,
	scope vfilter.Scope,
	args *ordereddict.Dict) <-chan vfilter.Row {
	output_chan := make(chan vfilter.Row)

	go func() {
		defer close(output_chan)

		arg := &ParseCSVPluginArgs{}
		err := arg_parser.ExtractArgsWithContext(ctx, scope, args, arg)
		if err != nil {
			scope.Log("watch_csv: %s", err.Error())
			return
		}

		err = vql_subsystem.CheckFilesystemAccess(scope, arg.Accessor)
		if err != nil {
			scope.Log("watch_csv: %s", err)
			return
		}

		event_channel := make(chan vfilter.Row)

		// Register the output channel as a listener to the
		// global event.
		for _, filename := range arg.Filenames {
			GlobalCSVService.Register(
				filename, arg.Accessor,
				ctx, scope, event_channel)
		}

		// Wait until the query is complete.
		for event := range event_channel {
			select {
			case <-ctx.Done():
				return

			case output_chan <- event:
			}
		}
	}()

	return output_chan
}

func (self _WatchCSVPlugin) Info(scope vfilter.Scope, type_map *vfilter.TypeMap) *vfilter.PluginInfo {
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
	scope vfilter.Scope,
	args *ordereddict.Dict) <-chan vfilter.Row {
	output_chan := make(chan vfilter.Row)

	go func() {
		defer close(output_chan)

		arg := &WriteCSVPluginArgs{}
		err := arg_parser.ExtractArgsWithContext(ctx, scope, args, arg)
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

		default:
			scope.Log("write_csv: Unsupported accessor for writing %v", arg.Accessor)
			return
		}

		for row := range arg.Query.Eval(ctx, scope) {
			writer.Write(row)
			select {
			case <-ctx.Done():
				return

			case output_chan <- row:
			}
		}
	}()

	return output_chan
}

func (self WriteCSVPlugin) Info(scope vfilter.Scope, type_map *vfilter.TypeMap) *vfilter.PluginInfo {
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
