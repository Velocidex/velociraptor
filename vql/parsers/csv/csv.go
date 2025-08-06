/*
Velociraptor - Dig Deeper
Copyright (C) 2019-2025 Rapid7 Inc.

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
	"errors"
	"fmt"
	"io"
	"os"

	"github.com/Velocidex/ordereddict"
	"www.velocidex.com/golang/velociraptor/accessors"
	"www.velocidex.com/golang/velociraptor/accessors/file"
	"www.velocidex.com/golang/velociraptor/acls"
	"www.velocidex.com/golang/velociraptor/config"
	"www.velocidex.com/golang/velociraptor/file_store/csv"
	"www.velocidex.com/golang/velociraptor/json"
	"www.velocidex.com/golang/velociraptor/vql"
	vql_subsystem "www.velocidex.com/golang/velociraptor/vql"
	vfilter "www.velocidex.com/golang/vfilter"
	"www.velocidex.com/golang/vfilter/arg_parser"
)

type ParseCSVPluginArgs struct {
	Filenames   []*accessors.OSPath `vfilter:"required,field=filename,doc=CSV files to open"`
	Accessor    string              `vfilter:"optional,field=accessor,doc=The accessor to use"`
	AutoHeaders bool                `vfilter:"optional,field=auto_headers,doc=If unset the first row is headers"`
	Separator   string              `vfilter:"optional,field=separator,doc=Comma separator (default ',')"`
	Comment     string              `vfilter:"optional,field=comment,doc=The single character that should be considered a comment"`
	Columns     []string            `vfilter:"optional,field=columns,doc=The columns to use"`
}

type ParseCSVPlugin struct{}

func (self ParseCSVPlugin) Call(
	ctx context.Context,
	scope vfilter.Scope,
	args *ordereddict.Dict) <-chan vfilter.Row {
	output_chan := make(chan vfilter.Row)

	go func() {
		defer close(output_chan)
		defer vql_subsystem.RegisterMonitor(ctx, "parse_csv", args)()

		arg := &ParseCSVPluginArgs{}
		err := arg_parser.ExtractArgsWithContext(ctx, scope, args, arg)
		if err != nil {
			scope.Log("parse_csv: %s", err.Error())
			return
		}

		for _, filename := range arg.Filenames {
			func() {
				accessor, err := accessors.GetAccessor(arg.Accessor, scope)
				if err != nil {
					scope.Log("parse_csv: %v", err)
					return
				}
				fd, err := accessor.OpenWithOSPath(filename)
				if err != nil {
					scope.Log("Unable to open file %s: %v",
						filename, err)
					return
				}
				defer fd.Close()

				csv_reader, err := csv.NewReader(fd)
				if err != nil {
					scope.Log("parse_csv: %v", err)
					return
				}
				csv_reader.TrimLeadingSpace = true
				csv_reader.LazyQuotes = true

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
					before_offset := csv_reader.ByteOffset
					row := ordereddict.NewDict()
					row_data, err := csv_reader.ReadAny()
					if errors.Is(err, io.EOF) {
						return
					}

					if err != nil {
						// Report the error and skip to the next record
						scope.Log("INFO:parse_csv: %v", err)

						// If we are not making any progress, just give up
						if csv_reader.ByteOffset == before_offset {
							return
						}
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
		Name:     "parse_csv",
		Doc:      "Parses events from a CSV file.",
		ArgType:  type_map.AddType(scope, &ParseCSVPluginArgs{}),
		Metadata: vql.VQLMetadata().Permissions(acls.FILESYSTEM_READ).Build(),
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
		defer vql_subsystem.RegisterMonitor(ctx, "watch_csv", args)()

		arg := &ParseCSVPluginArgs{}
		err := arg_parser.ExtractArgsWithContext(ctx, scope, args, arg)
		if err != nil {
			scope.Log("watch_csv: %s", err.Error())
			return
		}

		event_channel := make(chan vfilter.Row)

		config_obj, ok := vql_subsystem.GetServerConfig(scope)
		if !ok {
			config_obj = config.GetDefaultConfig()
		}

		// Register the output channel as a listener to the
		// global event.
		for _, filename := range arg.Filenames {
			watcher_service := NewCSVWatcherService(config_obj)
			watcher_service.Register(
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
		ArgType:  type_map.AddType(scope, &ParseCSVPluginArgs{}),
		Metadata: vql.VQLMetadata().Permissions(acls.FILESYSTEM_READ).Build(),
	}
}

type WriteCSVPluginArgs struct {
	Filename *accessors.OSPath   `vfilter:"required,field=filename,doc=CSV files to open"`
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
		defer vql_subsystem.RegisterMonitor(ctx, "write_csv", args)()

		arg := &WriteCSVPluginArgs{}
		err := arg_parser.ExtractArgsWithContext(ctx, scope, args, arg)
		if err != nil {
			scope.Log("write_csv: %s", err.Error())
			return
		}

		var writer *csv.CSVWriter

		switch arg.Accessor {
		case "", "auto", "file":
			err := vql_subsystem.CheckAccess(scope, acls.FILESYSTEM_WRITE)
			if err != nil {
				scope.Log("write_csv: %s", err)
				return
			}

			// Make sure we are allowed to write there.
			err = file.CheckPrefix(arg.Filename)
			if err != nil {
				scope.Log("write_csv: %v", err)
				return
			}

			underlying_file, err := accessors.GetUnderlyingAPIFilename(
				arg.Accessor, scope, arg.Filename)
			if err != nil {
				scope.Log("write_csv: %s", err)
				return
			}

			file, err := os.OpenFile(underlying_file,
				os.O_RDWR|os.O_CREATE|os.O_TRUNC, 0700)
			if err != nil {
				scope.Log("write_csv: Unable to open file %s: %s",
					arg.Filename, err.Error())
				return
			}
			defer file.Close()

			config_obj, _ := vql_subsystem.GetServerConfig(scope)
			writer = csv.GetCSVAppender(
				config_obj, scope, file, true, json.DefaultEncOpts())
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
		Name:     "write_csv",
		Doc:      "Write a query into a CSV file.",
		ArgType:  type_map.AddType(scope, &WriteCSVPluginArgs{}),
		Metadata: vql.VQLMetadata().Permissions(acls.FILESYSTEM_WRITE).Build(),
	}
}

func init() {
	vql_subsystem.RegisterPlugin(&ParseCSVPlugin{})
	vql_subsystem.RegisterPlugin(&_WatchCSVPlugin{})
	vql_subsystem.RegisterPlugin(&WriteCSVPlugin{})
}
