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
// A simple line based file parser with common separator. This could
// be done with "parse_with_regex" but its easier to have a dedicated
// parser.
package parsers

import (
	"bufio"
	"context"
	"fmt"
	"regexp"

	"github.com/Velocidex/ordereddict"
	glob "www.velocidex.com/golang/velociraptor/glob"
	utils "www.velocidex.com/golang/velociraptor/utils"
	vql_subsystem "www.velocidex.com/golang/velociraptor/vql"
	"www.velocidex.com/golang/vfilter"
	"www.velocidex.com/golang/vfilter/arg_parser"
)

var (
	sanitize_re = regexp.MustCompile("[^a-zA-Z0-9]")
)

type _SplitRecordParserArgs struct {
	Filenames            []string `vfilter:"required,field=filenames,doc=Files to parse."`
	Accessor             string   `vfilter:"optional,field=accessor,doc=The accessor to use"`
	Regex                string   `vfilter:"required,field=regex,doc=The split regular expression (e.g. a comma)"`
	compiled_regex       *regexp.Regexp
	Columns              []string `vfilter:"optional,field=columns,doc=If the first row is not the headers, this arg must provide a list of column names for each value."`
	First_row_is_headers bool     `vfilter:"optional,field=first_row_is_headers,doc=A bool indicating if we should get column names from the first row."`
	Count                int      `vfilter:"optional,field=count,doc=Only split into this many columns if possible."`
}

type _SplitRecordParser struct{}

func processFile(
	ctx context.Context,
	scope vfilter.Scope,
	file string, arg *_SplitRecordParserArgs,
	output_chan chan vfilter.Row) {

	err := vql_subsystem.CheckFilesystemAccess(scope, arg.Accessor)
	if err != nil {
		scope.Log("split_records: %s", err)
		return
	}

	accessor, err := glob.GetAccessor(arg.Accessor, scope)
	if err != nil {
		scope.Log("split_records: %v", err)
		return
	}
	fd, err := accessor.Open(file)
	if err != nil {
		scope.Log("split_records: %v", err)
		return
	}
	defer fd.Close()

	reader := bufio.NewReader(fd)
	for {
		select {
		case <-ctx.Done():
			return

		default:
			line, err := reader.ReadString('\n')
			if err != nil {
				return
			}

			if arg.Count == 0 {
				arg.Count = -1
			}
			items := arg.compiled_regex.Split(line, arg.Count)
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
		}
	}
}

func (self _SplitRecordParser) Call(
	ctx context.Context,
	scope vfilter.Scope,
	args *ordereddict.Dict) <-chan vfilter.Row {
	output_chan := make(chan vfilter.Row)
	var compiled_regex *regexp.Regexp

	arg := _SplitRecordParserArgs{}
	err := arg_parser.ExtractArgsWithContext(ctx, scope, args, &arg)
	if err != nil {
		goto error
	}

	if arg.Regex == "" {
		arg.Regex = "\\s+"
	}

	compiled_regex, err = regexp.Compile(arg.Regex)
	if err != nil {
		goto error
	}
	arg.compiled_regex = compiled_regex

	go func() {
		defer close(output_chan)

		for _, file := range arg.Filenames {
			select {
			case <-ctx.Done():
				return

			default:
				processFile(ctx, scope, file, &arg, output_chan)
			}
		}
	}()

	return output_chan

error:
	scope.Log("%s: %s", self.Name(), err.Error())
	close(output_chan)
	return output_chan

}

func (self _SplitRecordParser) Name() string {
	return "split_records"
}

func (self _SplitRecordParser) Info(scope vfilter.Scope, type_map *vfilter.TypeMap) *vfilter.PluginInfo {
	return &vfilter.PluginInfo{
		Name:    "split_records",
		Doc:     "Parses files by splitting lines into records.",
		ArgType: type_map.AddType(scope, &_SplitRecordParserArgs{}),
	}
}

func init() {
	vql_subsystem.RegisterPlugin(&_SplitRecordParser{})

}
