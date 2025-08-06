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
// A simple line based file parser with common separator. This could
// be done with "parse_with_regex" but its easier to have a dedicated
// parser.
package parsers

import (
	"context"
	"fmt"
	"regexp"

	"github.com/Velocidex/ordereddict"
	"www.velocidex.com/golang/velociraptor/accessors"
	"www.velocidex.com/golang/velociraptor/acls"
	utils "www.velocidex.com/golang/velociraptor/utils"
	"www.velocidex.com/golang/velociraptor/vql"
	vql_subsystem "www.velocidex.com/golang/velociraptor/vql"
	"www.velocidex.com/golang/vfilter"
	"www.velocidex.com/golang/vfilter/arg_parser"
)

var (
	sanitize_re = regexp.MustCompile("[^a-zA-Z0-9]")
)

type _SplitRecordParserArgs struct {
	Filenames            []*accessors.OSPath `vfilter:"required,field=filenames,doc=Files to parse."`
	Accessor             string              `vfilter:"optional,field=accessor,doc=The accessor to use"`
	Regex                string              `vfilter:"optional,field=regex,doc=The split regular expression (e.g. a comma, default whitespace)"`
	Columns              []string            `vfilter:"optional,field=columns,doc=If the first row is not the headers, this arg must provide a list of column names for each value."`
	First_row_is_headers bool                `vfilter:"optional,field=first_row_is_headers,doc=A bool indicating if we should get column names from the first row."`
	Count                int                 `vfilter:"optional,field=count,doc=Only split into this many columns if possible."`
	RecordRegex          string              `vfilter:"optional,field=record_regex,doc=A regex to split data into records (default \n)"`
	BufferSize           int                 `vfilter:"optional,field=buffer_size,doc=Maximum size of line buffer (default 64kb)."`
}

type SplitRecordParser struct{}

// Prepare for the next buffer - slide the buffer after the
// end of the match.
func slideBuffer(buffer []byte, start, end int) int {
	i := 0
	j := start
	for j < end {
		buffer[i] = buffer[j]
		i++
		j++
	}
	return i
}

func processFile(
	ctx context.Context,
	scope vfilter.Scope,
	file *accessors.OSPath,
	compiled_regex *regexp.Regexp,
	line_regex *regexp.Regexp,
	arg *_SplitRecordParserArgs,
	output_chan chan vfilter.Row) {

	accessor, err := accessors.GetAccessor(arg.Accessor, scope)
	if err != nil {
		scope.Log("split_records: %v", err)
		return
	}

	fd, err := accessor.OpenWithOSPath(file)
	if err != nil {
		scope.Log("split_records: %v", err)
		return
	}
	defer fd.Close()

	if arg.BufferSize == 0 {
		arg.BufferSize = 64 * 1024
	}

	buffer := make([]byte, arg.BufferSize)
	offset := 0
	for {
		n, _ := fd.Read(buffer[offset:])
		if offset == 0 && n == 0 {
			return
		}

		end := n + offset
		// Find the next line
		match := line_regex.FindIndex(buffer[:end])
		var line string
		if match == nil {
			// Separator is not found in the buffer, assume the whole
			// thing matches.
			match = []int{end, end}
		}

		// The line goes to the start of the line match
		line = string(buffer[:match[0]])

		if arg.Count == 0 {
			arg.Count = -1
		}

		items := compiled_regex.Split(line, arg.Count)
		// Need to make new columns.
		if len(arg.Columns) == 0 {
			if arg.First_row_is_headers {
				count := 1
				for _, item := range items {
					if utils.InString(arg.Columns, item) {
						item = fmt.Sprintf("%s%d",
							item, count)
						count += 1
					}

					item := sanitize_re.ReplaceAllLiteralString(item, "_")
					arg.Columns = append(arg.Columns, item)
				}
				offset = slideBuffer(buffer, match[1], end)
				continue
			}

			for idx := range items {
				arg.Columns = append(
					arg.Columns,
					fmt.Sprintf("Column%d", idx))
			}
		}
		result := ordereddict.NewDict()
		for idx, column := range arg.Columns {
			if idx < len(items) {
				result.Set(column, items[idx])
			} else {
				result.Set(column, vfilter.Null{})
			}
		}
		select {
		case <-ctx.Done():
			return

		case output_chan <- result:
		}

		offset = slideBuffer(buffer, match[1], end)
	}
}

func (self SplitRecordParser) Call(
	ctx context.Context,
	scope vfilter.Scope,
	args *ordereddict.Dict) <-chan vfilter.Row {
	output_chan := make(chan vfilter.Row)

	go func() {
		defer close(output_chan)
		defer vql_subsystem.RegisterMonitor(ctx, "split_records", args)()

		arg := _SplitRecordParserArgs{}
		err := arg_parser.ExtractArgsWithContext(ctx, scope, args, &arg)
		if err != nil {
			scope.Log("%s: %v", self.Name(), err)
			return
		}

		if arg.Regex == "" {
			arg.Regex = `\s+`
		}

		if arg.RecordRegex == "" {
			arg.RecordRegex = `\n`
		}

		compiled_regex, err := regexp.Compile(arg.Regex)
		if err != nil {
			scope.Log("%s: %v", self.Name(), err)
			return
		}

		line_regex, err := regexp.Compile(arg.RecordRegex)
		if err != nil {
			scope.Log("%s: %v", self.Name(), err)
			return
		}

		for _, file := range arg.Filenames {
			select {
			case <-ctx.Done():
				return

			default:
				processFile(ctx, scope, file, compiled_regex,
					line_regex, &arg, output_chan)
			}
		}
	}()

	return output_chan
}

func (self SplitRecordParser) Name() string {
	return "split_records"
}

func (self SplitRecordParser) Info(scope vfilter.Scope, type_map *vfilter.TypeMap) *vfilter.PluginInfo {
	return &vfilter.PluginInfo{
		Name:     "split_records",
		Doc:      "Parses files by splitting lines into records.",
		ArgType:  type_map.AddType(scope, &_SplitRecordParserArgs{}),
		Metadata: vql.VQLMetadata().Permissions(acls.FILESYSTEM_READ).Build(),
	}
}

func init() {
	vql_subsystem.RegisterPlugin(&SplitRecordParser{})

}
