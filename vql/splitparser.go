// A simple line based file parser with common separator. This could
// be done with "parse_with_regex" but its easier to have a dedicated
// parser.
package vql

import (
	"bufio"
	"context"
	"fmt"
	"regexp"
	glob "www.velocidex.com/golang/velociraptor/glob"
	utils "www.velocidex.com/golang/velociraptor/utils"
	"www.velocidex.com/golang/vfilter"
)

var (
	sanitize_re = regexp.MustCompile("[^a-zA-Z0-9]")
)

type _SplitRecordParserArgs struct {
	Filenames            []string `vfilter:"required,field=filenames"`
	Regex                string   `vfilter:"required,field=regex"`
	compiled_regex       *regexp.Regexp
	Columns              []string `vfilter:"optional,field=columns"`
	First_row_is_headers bool     `vfilter:"optional,field=first_row_is_headers"`
}

type _SplitRecordParser struct{}

func processFile(
	ctx context.Context,
	file string, arg *_SplitRecordParserArgs,
	output_chan chan vfilter.Row) {

	accessor := glob.OSFileSystemAccessor{}
	fd, err := accessor.Open(file)
	if err != nil {
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

			items := arg.compiled_regex.Split(line, -1)
			// Need to make new columns.
			if len(arg.Columns) == 0 {
				if arg.First_row_is_headers {
					count := 1
					for _, item := range items {
						if utils.InString(&arg.Columns, item) {
							item = fmt.Sprintf("%s%d",
								item, count)
							count += 1
						}

						item := sanitize_re.ReplaceAllLiteralString(item, "_")
						arg.Columns = append(arg.Columns, item)
					}
					continue
				}

				for idx, _ := range items {
					arg.Columns = append(
						arg.Columns,
						fmt.Sprintf("Column%d", idx))
				}
			}
			result := vfilter.NewDict()
			for idx, column := range arg.Columns {
				if idx < len(items) {
					result.Set(column, items[idx])
				}
			}
			output_chan <- result
		}
	}
}

func (self _SplitRecordParser) Call(
	ctx context.Context,
	scope *vfilter.Scope,
	args *vfilter.Dict) <-chan vfilter.Row {
	output_chan := make(chan vfilter.Row)
	var compiled_regex *regexp.Regexp

	arg := _SplitRecordParserArgs{}
	err := vfilter.ExtractArgs(scope, args, &arg)
	if err != nil {
		goto error
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
				processFile(ctx, file, &arg, output_chan)
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

func (self _SplitRecordParser) Info(type_map *vfilter.TypeMap) *vfilter.PluginInfo {
	return &vfilter.PluginInfo{
		Name: "split_records",
		Doc:  "Parses files by splitting lines into records.",
	}
}

func init() {
	exportedPlugins = append(exportedPlugins, &_SplitRecordParser{})

}
