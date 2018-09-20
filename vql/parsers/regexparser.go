package parsers

import (
	"context"
	"fmt"
	"regexp"

	"www.velocidex.com/golang/velociraptor/glob"
	utils "www.velocidex.com/golang/velociraptor/utils"
	vql_subsystem "www.velocidex.com/golang/velociraptor/vql"
	"www.velocidex.com/golang/vfilter"
)

type _ParseFileWithRegexArgs struct {
	Filenames       []string `vfilter:"required,field=file"`
	Regex           []string `vfilter:"required,field=regex"`
	Accessor        string   `vfilter:"optional,field=accessor"`
	compiled_regexs []*regexp.Regexp
	capture_vars    []string
}

type _ParseFileWithRegex struct{}

func _ParseFile(
	ctx context.Context,
	filename string,
	scope *vfilter.Scope,
	arg *_ParseFileWithRegexArgs,
	output_chan chan vfilter.Row) {
	accessor := glob.GetAccessor(arg.Accessor, ctx)
	file, err := accessor.Open(filename)
	if err != nil {
		scope.Log("Unable to open file %s", filename)
		return
	}
	defer file.Close()

	buffer := make([]byte, 40960)
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
					row := vfilter.NewDict().Set(
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
					output_chan <- row
				}
			}
		}

	}

}

func (self _ParseFileWithRegex) Call(
	ctx context.Context,
	scope *vfilter.Scope,
	args *vfilter.Dict) <-chan vfilter.Row {
	output_chan := make(chan vfilter.Row)
	arg := &_ParseFileWithRegexArgs{}
	err := vfilter.ExtractArgs(scope, args, arg)
	if err != nil {
		scope.Log("parse_records_with_regex: %s", err.Error())
		close(output_chan)
		return output_chan
	}

	for _, regex := range arg.Regex {
		r, err := regexp.Compile(regex)
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
			if !utils.InString(&arg.capture_vars, x) && x != "" {
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

func (self _ParseFileWithRegex) Info(type_map *vfilter.TypeMap) *vfilter.PluginInfo {
	return &vfilter.PluginInfo{
		Name:    "parse_records_with_regex",
		Doc:     "Parses a file with a set of regexp and yields matches as records.",
		ArgType: type_map.AddType(&_ParseFileWithRegexArgs{}),
	}
}

type _ParseStringWithRegexFunctionArgs struct {
	String string   `vfilter:"required,field=string"`
	Regex  []string `vfilter:"required,field=regex"`
}

type _ParseStringWithRegexFunction struct{}

func (self *_ParseStringWithRegexFunction) Call(ctx context.Context,
	scope *vfilter.Scope,
	args *vfilter.Dict) (result vfilter.Any) {
	arg := &_ParseStringWithRegexFunctionArgs{}
	err := vfilter.ExtractArgs(scope, args, arg)
	if err != nil {
		scope.Log("parse_string_with_regex: %s", err.Error())
		return vfilter.Null{}
	}
	row := vfilter.NewDict()
	merged_names := []string{}
	for _, regex := range arg.Regex {
		r, err := regexp.Compile(regex)
		if err != nil {
			scope.Log("Unable to compile regex %s", regex)
			return vfilter.Null{}
		}

		match := r.FindAllStringSubmatch(arg.String, -1)
		if match != nil {
			names := r.SubexpNames()
			for _, x := range names {
				if !utils.InString(&merged_names, x) && x != "" {
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

func (self _ParseStringWithRegexFunction) Info(type_map *vfilter.TypeMap) *vfilter.FunctionInfo {
	return &vfilter.FunctionInfo{
		Name: "parse_string_with_regex",
		Doc: "Parse a string with a set of regex and extract fields. Returns " +
			"a dict with fields populated from all regex capture variables.",
		ArgType: type_map.AddType(&_ParseStringWithRegexFunctionArgs{}),
	}
}

type _RegexReplaceArg struct {
	Source  string `vfilter:"required,field=source"`
	Replace string `vfilter:"required,field=replace"`
	Re      string `vfilter:"required,field=re"`
}

type _RegexReplace struct{}

func (self _RegexReplace) Call(
	ctx context.Context,
	scope *vfilter.Scope,
	args *vfilter.Dict) vfilter.Any {
	arg := &_RegexReplaceArg{}
	err := vfilter.ExtractArgs(scope, args, arg)
	if err != nil {
		scope.Log("regex_replace: %s", err.Error())
		return vfilter.Null{}
	}
	re, err := regexp.Compile(arg.Re)
	if err != nil {
		scope.Log("Unable to compile regex %s", arg.Re)
		return vfilter.Null{}
	}

	return re.ReplaceAllString(arg.Source, arg.Replace)
}

func (self _RegexReplace) Info(type_map *vfilter.TypeMap) *vfilter.FunctionInfo {
	return &vfilter.FunctionInfo{
		Name:    "regex_replace",
		Doc:     "Search and replace a string with a regexp.",
		ArgType: type_map.AddType(&_RegexReplaceArg{}),
	}
}

func init() {
	vql_subsystem.RegisterPlugin(&_ParseFileWithRegex{})
	vql_subsystem.RegisterFunction(&_ParseStringWithRegexFunction{})
	vql_subsystem.RegisterFunction(&_RegexReplace{})
}
