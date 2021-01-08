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
	"fmt"
	"regexp"
	"sync"

	"github.com/Velocidex/ordereddict"
	"www.velocidex.com/golang/velociraptor/glob"
	utils "www.velocidex.com/golang/velociraptor/utils"
	vql_subsystem "www.velocidex.com/golang/velociraptor/vql"
	"www.velocidex.com/golang/vfilter"
)

const BUFF_SIZE = 40960

var (
	pool = sync.Pool{
		New: func() interface{} {
			buffer := make([]byte, BUFF_SIZE)
			return &buffer
		},
	}
)

type _ParseFileWithRegexArgs struct {
	Filenames       []string `vfilter:"required,field=file,doc=A list of files to parse."`
	Regex           []string `vfilter:"required,field=regex,doc=A list of regex to apply to the file data."`
	Accessor        string   `vfilter:"optional,field=accessor,doc=The accessor to use."`
	compiled_regexs []*regexp.Regexp
	capture_vars    []string
}

type _ParseFileWithRegex struct{}

func _ParseFile(
	ctx context.Context,
	filename string,
	scope vfilter.Scope,
	arg *_ParseFileWithRegexArgs,
	output_chan chan vfilter.Row) {

	err := vql_subsystem.CheckFilesystemAccess(scope, arg.Accessor)
	if err != nil {
		scope.Log("parse_records_with_regex: %s", err)
		return
	}

	accessor, err := glob.GetAccessor(arg.Accessor, scope)
	if err != nil {
		scope.Log("error: %v", err)
		return
	}

	file, err := accessor.Open(filename)
	if err != nil {
		scope.Log("Unable to open file %s", filename)
		return
	}
	defer file.Close()

	cached_buffer := pool.Get().(*[]byte)
	defer pool.Put(cached_buffer)

	buffer := *cached_buffer

	for {
		n, _ := file.Read(buffer)
		if n == 0 {
			return
		}

		for _, r := range arg.compiled_regexs {
			match := r.FindAllSubmatch(buffer[:n], -1)
			if match != nil {
				names := r.SubexpNames()
				for _, hit := range match {
					row := ordereddict.NewDict().Set(
						"FullPath", filename)
					for _, name := range arg.capture_vars {
						if name != "" {
							row.Set(name, "")
						}
					}
					for idx, submatch := range hit {
						if idx == 0 {
							continue
						}

						key := fmt.Sprintf("g%d", idx)
						if names[idx] != "" {
							key = names[idx]
						}

						row.Set(key, string(submatch))
					}
					select {
					case <-ctx.Done():
						return

					case output_chan <- row:
					}
				}
			}
		}

	}

}

func (self _ParseFileWithRegex) Call(
	ctx context.Context,
	scope vfilter.Scope,
	args *ordereddict.Dict) <-chan vfilter.Row {
	output_chan := make(chan vfilter.Row)
	arg := &_ParseFileWithRegexArgs{}
	err := vfilter.ExtractArgs(scope, args, arg)
	if err != nil {
		scope.Log("parse_records_with_regex: %s", err.Error())
		close(output_chan)
		return output_chan
	}

	for _, regex := range arg.Regex {
		r, err := regexp.Compile("(?i)" + regex)
		if err != nil {
			scope.Log("Unable to compile regex %s", regex)
			close(output_chan)
			return output_chan
		}
		arg.compiled_regexs = append(arg.compiled_regexs, r)

		// Collect all the capture vars from all the regex. We
		// make sure the result row has something in each
		// position to avoid errors.
		for _, x := range r.SubexpNames() {
			if !utils.InString(arg.capture_vars, x) && x != "" {
				arg.capture_vars = append(arg.capture_vars, x)
			}
		}
	}

	go func() {
		defer close(output_chan)

		for _, filename := range arg.Filenames {
			_ParseFile(ctx, filename, scope, arg, output_chan)
		}
	}()

	return output_chan
}

func (self _ParseFileWithRegex) Info(scope vfilter.Scope, type_map *vfilter.TypeMap) *vfilter.PluginInfo {
	return &vfilter.PluginInfo{
		Name:    "parse_records_with_regex",
		Doc:     "Parses a file with a set of regexp and yields matches as records.",
		ArgType: type_map.AddType(scope, &_ParseFileWithRegexArgs{}),
	}
}

type _ParseStringWithRegexFunctionArgs struct {
	String string   `vfilter:"required,field=string,doc=A string to parse."`
	Regex  []string `vfilter:"required,field=regex,doc=The regex to apply."`
}

type _ParseStringWithRegexFunction struct{}

func (self *_ParseStringWithRegexFunction) Call(ctx context.Context,
	scope vfilter.Scope,
	args *ordereddict.Dict) (result vfilter.Any) {
	arg := &_ParseStringWithRegexFunctionArgs{}
	err := vfilter.ExtractArgs(scope, args, arg)
	if err != nil {
		scope.Log("parse_string_with_regex: %s", err.Error())
		return vfilter.Null{}
	}
	row := ordereddict.NewDict()
	merged_names := []string{}
	for _, regex := range arg.Regex {
		r, err := regexp.Compile("(?i)" + regex)
		if err != nil {
			scope.Log("Unable to compile regex %s", regex)
			return vfilter.Null{}
		}

		match := r.FindAllStringSubmatch(arg.String, -1)
		if match != nil {
			names := r.SubexpNames()
			for _, x := range names {
				if !utils.InString(merged_names, x) && x != "" {
					merged_names = append(merged_names, x)
				}
			}
			for _, hit := range match {
				for idx, submatch := range hit {
					if idx == 0 {
						continue
					}
					key := fmt.Sprintf("g%d", idx)
					if names[idx] != "" {
						key = names[idx]
					}

					row.Set(key, string(submatch))
				}
			}
		}
	}

	for _, name := range merged_names {
		_, pres := row.Get(name)
		if !pres {
			row.Set(name, "")
		}
	}

	return row
}

func (self _ParseStringWithRegexFunction) Info(scope vfilter.Scope, type_map *vfilter.TypeMap) *vfilter.FunctionInfo {
	return &vfilter.FunctionInfo{
		Name: "parse_string_with_regex",
		Doc: "Parse a string with a set of regex and extract fields. Returns " +
			"a dict with fields populated from all regex capture variables.",
		ArgType: type_map.AddType(scope, &_ParseStringWithRegexFunctionArgs{}),
	}
}

type _RegexReplaceArg struct {
	Source  string `vfilter:"required,field=source,doc=The source string to replace."`
	Replace string `vfilter:"required,field=replace,doc=The substitute string."`
	Re      string `vfilter:"required,field=re,doc=A regex to apply"`
}

type _RegexReplace struct{}

func (self _RegexReplace) Call(
	ctx context.Context,
	scope vfilter.Scope,
	args *ordereddict.Dict) vfilter.Any {
	arg := &_RegexReplaceArg{}
	err := vfilter.ExtractArgs(scope, args, arg)
	if err != nil {
		scope.Log("regex_replace: %s", err.Error())
		return vfilter.Null{}
	}
	re, err := regexp.Compile("(?i)" + arg.Re)
	if err != nil {
		scope.Log("Unable to compile regex %s", arg.Re)
		return vfilter.Null{}
	}

	return re.ReplaceAllString(arg.Source, arg.Replace)
}

func (self _RegexReplace) Info(scope vfilter.Scope, type_map *vfilter.TypeMap) *vfilter.FunctionInfo {
	return &vfilter.FunctionInfo{
		Name: "regex_replace",
		Doc: "Search and replace a string with a regexp. " +
			"Note you can use $1 to replace the capture string.",
		ArgType: type_map.AddType(scope, &_RegexReplaceArg{}),
	}
}

func init() {
	vql_subsystem.RegisterPlugin(&_ParseFileWithRegex{})
	vql_subsystem.RegisterFunction(&_ParseStringWithRegexFunction{})
	vql_subsystem.RegisterFunction(&_RegexReplace{})
}
