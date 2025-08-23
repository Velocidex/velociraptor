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
	"www.velocidex.com/golang/vfilter/types"
)

type _ParseFileWithRegexArgs struct {
	Filenames       []*accessors.OSPath `vfilter:"required,field=file,doc=A list of files to parse."`
	Regex           []string            `vfilter:"required,field=regex,doc=A list of regex to apply to the file data."`
	Accessor        string              `vfilter:"optional,field=accessor,doc=The accessor to use."`
	BufferSize      int                 `vfilter:"optional,field=buffer_size,doc=Maximum size of line buffer (default 64kb)."`
	compiled_regexs []*regexp.Regexp
	capture_vars    []string
}

type _ParseFileWithRegex struct{}

func _ParseFile(
	ctx context.Context,
	filename *accessors.OSPath,
	scope vfilter.Scope,
	arg *_ParseFileWithRegexArgs,
	output_chan chan vfilter.Row) {

	accessor, err := accessors.GetAccessor(arg.Accessor, scope)
	if err != nil {
		scope.Log("error: %v", err)
		return
	}

	file, err := accessor.OpenWithOSPath(filename)
	if err != nil {
		scope.Log("Unable to open file %s", filename)
		return
	}
	defer file.Close()

	if arg.BufferSize == 0 {
		arg.BufferSize = 64 * 1024
	}

	offset := 0
	buffer := make([]byte, arg.BufferSize)

next:
	for {
		n, _ := file.Read(buffer[offset:])
		if n == 0 && offset == 0 {
			return
		}

		end := n + offset

		for _, r := range arg.compiled_regexs {
			hits := r.FindSubmatchIndex(buffer[:end])

			// No matches in this buffer, try the next regex
			if len(hits) < 2 {
				continue
			}

			names := r.SubexpNames()
			row := ordereddict.NewDict().Set("FullPath", filename)
			for _, name := range arg.capture_vars {
				if name != "" {
					row.Set(name, "")
				}
			}

			// Get all capture variables
			if len(hits) >= 2 && len(hits)%2 == 0 {
				for i := 2; i < len(hits); i += 2 {
					start := hits[i]
					end := hits[i+1]
					if start < 0 || end < 0 {
						continue
					}

					idx := i / 2
					key := fmt.Sprintf("g%d", idx)
					if names[idx] != "" {
						key = names[idx]
					}
					row.Set(key, string(buffer[start:end]))
				}
			}

			select {
			case <-ctx.Done():
				return

			case output_chan <- row:
				// Slide the buffer for the next match
				offset = slideBuffer(buffer, hits[1], end)
				continue next
			}
		}

		// If we get here none of the regex matched the buffer, wipe
		// the buffer and start again
		offset = 0
	}

}

func (self _ParseFileWithRegex) Call(
	ctx context.Context,
	scope vfilter.Scope,
	args *ordereddict.Dict) <-chan vfilter.Row {
	output_chan := make(chan vfilter.Row)

	defer vql_subsystem.RegisterMonitor(ctx, "parse_records_with_regex", args)()

	arg := &_ParseFileWithRegexArgs{}
	err := arg_parser.ExtractArgsWithContext(ctx, scope, args, arg)
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
		defer vql_subsystem.RegisterMonitor(ctx, "parse_records_with_regex", args)()

		for _, filename := range arg.Filenames {
			_ParseFile(ctx, filename, scope, arg, output_chan)
		}
	}()

	return output_chan
}

func (self _ParseFileWithRegex) Info(scope vfilter.Scope, type_map *vfilter.TypeMap) *vfilter.PluginInfo {
	return &vfilter.PluginInfo{
		Name:     "parse_records_with_regex",
		Doc:      "Parses a file with a set of regexp and yields matches as records.",
		ArgType:  type_map.AddType(scope, &_ParseFileWithRegexArgs{}),
		Metadata: vql.VQLMetadata().Permissions(acls.FILESYSTEM_READ).Build(),
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

	defer vql_subsystem.RegisterMonitor(ctx, "parse_string_with_regex", args)()

	arg := &_ParseStringWithRegexFunctionArgs{}
	err := arg_parser.ExtractArgsWithContext(ctx, scope, args, arg)
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
	Source        string `vfilter:"required,field=source,doc=The source string to replace."`
	Replace       string `vfilter:"optional,field=replace,doc=The substitute string."`
	ReplaceLambda string `vfilter:"optional,field=replace_lambda,doc=Optionally the replacement can be a lambda."`
	Re            string `vfilter:"required,field=re,doc=A regex to apply"`
}

type _RegexReplace struct{}

func (self _RegexReplace) Call(
	ctx context.Context,
	scope vfilter.Scope,
	args *ordereddict.Dict) vfilter.Any {

	defer vql_subsystem.RegisterMonitor(ctx, "regex_replace", args)()

	arg := &_RegexReplaceArg{}
	err := arg_parser.ExtractArgsWithContext(ctx, scope, args, arg)
	if err != nil {
		scope.Log("regex_replace: %v", err)
		return vfilter.Null{}
	}
	re, err := regexp.Compile("(?i)" + arg.Re)
	if err != nil {
		scope.Log("Unable to compile regex %s", arg.Re)
		return vfilter.Null{}
	}

	if arg.ReplaceLambda != "" {
		var lambda *vfilter.Lambda

		cache_key := "re:" + arg.ReplaceLambda
		lambda_any := vql_subsystem.CacheGet(scope, cache_key)
		if lambda_any != nil {
			lambda, _ = lambda_any.(*vfilter.Lambda)
		}

		if lambda == nil {
			lambda, err = vfilter.ParseLambda(arg.ReplaceLambda)
			if err != nil {
				scope.Log("regex_replace: Unable to compile lambda %s",
					arg.ReplaceLambda)
				return vfilter.Null{}
			}
			vql_subsystem.CacheSet(scope, cache_key, lambda)
		}

		return re.ReplaceAllStringFunc(arg.Source,
			func(str string) string {
				return fmt.Sprintf("%v",
					lambda.Reduce(ctx, scope, []types.Any{str}))
			})
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

type _RegexMapArg struct {
	Source string            `vfilter:"required,field=source,doc=The source string to replace."`
	Map    *ordereddict.Dict `vfilter:"required,field=map,doc=A dict with keys reg, values substitutions."`
	Key    string            `vfilter:"optional,field=key,doc=A key for caching"`
}

type _Transform struct {
	search  *regexp.Regexp
	replace string
}

type _RegexMap struct{}

func (self _RegexMap) Call(
	ctx context.Context,
	scope vfilter.Scope,
	args *ordereddict.Dict) vfilter.Any {

	defer vql_subsystem.RegisterMonitor(ctx, "regex_transform", args)()

	arg := &_RegexMapArg{}
	err := arg_parser.ExtractArgsWithContext(ctx, scope, args, arg)
	if err != nil {
		scope.Log("regex_transform: %s", err.Error())
		return vfilter.Null{}
	}

	key := arg.Key
	if key == "" {
		key = arg.Map.String()
	}
	key = "$regex_map_" + key

	var transforms []*_Transform
	regex_map := vql_subsystem.CacheGet(scope, key)
	if utils.IsNil(regex_map) {
		// Make a new set of transforms
		for _, i := range arg.Map.Items() {
			search := i.Key
			replace := utils.ToString(i.Value)

			re, err := regexp.Compile("(?i)" + search)
			if err != nil {
				scope.Log("regex_transform: Unable to compile regex %s: %v", search, err)
				return vfilter.Null{}
			}

			transforms = append(transforms, &_Transform{
				search: re, replace: replace})
		}
		vql_subsystem.CacheSet(scope, key, transforms)
	} else {
		transforms, _ = regex_map.([]*_Transform)
		if transforms == nil {
			scope.Log("regex_transform: error recovering map from cache")
			return vfilter.Null{}
		}
	}

	source := arg.Source
	for _, transform := range transforms {
		source = transform.search.ReplaceAllString(source, transform.replace)
	}

	return source
}

func (self _RegexMap) Info(scope vfilter.Scope, type_map *vfilter.TypeMap) *vfilter.FunctionInfo {
	return &vfilter.FunctionInfo{
		Name: "regex_transform",
		Doc: "Search and replace a string with multiple regex. " +
			"Note you can use $1 to replace the capture string.",
		ArgType: type_map.AddType(scope, &_RegexMapArg{}),
	}
}

func init() {
	vql_subsystem.RegisterPlugin(&_ParseFileWithRegex{})
	vql_subsystem.RegisterFunction(&_ParseStringWithRegexFunction{})
	vql_subsystem.RegisterFunction(&_RegexReplace{})
	vql_subsystem.RegisterFunction(&_RegexMap{})
}
