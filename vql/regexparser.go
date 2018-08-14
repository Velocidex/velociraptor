package vql

import (
	"context"
	"fmt"
	"regexp"
	"www.velocidex.com/golang/velociraptor/glob"
	utils "www.velocidex.com/golang/velociraptor/utils"
	"www.velocidex.com/golang/vfilter"
)

type _ParseFileWithRegex struct{}

func _ParseFile(filename string,
	scope *vfilter.Scope,
	capture_vars []string,
	compiled_regexs []*regexp.Regexp,
	output_chan chan vfilter.Row) {
	accessor := glob.OSFileSystemAccessor{}
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

		for _, r := range compiled_regexs {
			match := r.FindAllSubmatch(buffer[:n], -1)
			if match != nil {
				names := r.SubexpNames()
				for _, hit := range match {
					row := vfilter.NewDict().Set(
						"FullPath", filename)
					for _, name := range capture_vars {
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

	filenames, ok := vfilter.ExtractStringArray(scope, "file", args)
	if !ok {
		scope.Log("Expecting string array as 'file' parameter")
		close(output_chan)
		return output_chan
	}

	regexps, ok := vfilter.ExtractStringArray(scope, "regex", args)
	if !ok {
		scope.Log("Expecting string list as 'regex' parameter")
		close(output_chan)
		return output_chan
	}

	capture_vars := []string{}
	var compiled_regexs []*regexp.Regexp
	for _, regex := range regexps {
		r, err := regexp.Compile(regex)
		if err != nil {
			scope.Log("Unable to compile regex %s", regex)
			close(output_chan)
			return output_chan
		}
		compiled_regexs = append(compiled_regexs, r)

		// Collect all the capture vars from all the regex. We
		// make sure the result row has something in each
		// position to avoid errors.
		for _, x := range r.SubexpNames() {
			if !utils.InString(&capture_vars, x) && x != "" {
				capture_vars = append(capture_vars, x)
			}
		}
	}

	go func() {
		defer close(output_chan)

		for _, filename := range filenames {
			_ParseFile(filename,
				scope,
				capture_vars,
				compiled_regexs,
				output_chan)
		}
	}()

	return output_chan
}

func (self _ParseFileWithRegex) Name() string {
	return "parse_with_regex"
}

func (self _ParseFileWithRegex) Info(type_map *vfilter.TypeMap) *vfilter.PluginInfo {
	return &vfilter.PluginInfo{
		Name: "parse_with_regex",
		Doc:  "Parses a file with a set of regexp and yields matches",
	}
}

type _ParseStringWithRegexFunction struct{}

func (self *_ParseStringWithRegexFunction) Call(ctx context.Context,
	scope *vfilter.Scope,
	args *vfilter.Dict) (result vfilter.Any) {
	data, ok := vfilter.ExtractString("string", args)
	if !ok {
		scope.Log("Expecting string as 'string' parameter")
		return vfilter.Null{}
	}

	regexes, ok := vfilter.ExtractStringArray(scope, "regex", args)
	if !ok {
		scope.Log("Expecting string array as 'regex' parameter")
		return vfilter.Null{}
	}

	row := vfilter.NewDict()
	merged_names := []string{}
	for _, regex := range regexes {
		r, err := regexp.Compile(regex)
		if err != nil {
			scope.Log("Unable to compile regex %s", regex)
			return vfilter.Null{}
		}

		match := r.FindAllStringSubmatch(*data, -1)
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

func (self _ParseStringWithRegexFunction) Name() string {
	return "parse_string_with_regex"
}

type _RegexReplace struct{}

func (self _RegexReplace) Call(
	ctx context.Context,
	scope *vfilter.Scope,
	args *vfilter.Dict) vfilter.Any {
	source, pres := vfilter.ExtractString("source", args)
	if !pres {
		scope.Log("Expected arg 'source' as string")
		return false
	}

	replace, pres := vfilter.ExtractString("replace", args)
	if !pres {
		scope.Log("Expected arg 'replace' as string")
		return false
	}

	regex, pres := vfilter.ExtractString("re", args)
	if !pres {
		scope.Log("Expected arg 're' as string")
		return false
	}

	re, err := regexp.Compile(*regex)
	if err != nil {
		scope.Log("Unable to compile regex %s", *regex)
		return vfilter.Null{}
	}

	return re.ReplaceAllString(*source, *replace)
}

func (self _RegexReplace) Name() string {
	return "regex_replace"
}

func init() {
	exportedPlugins = append(exportedPlugins, &_ParseFileWithRegex{})
	exportedFunctions = append(exportedFunctions,
		&_ParseStringWithRegexFunction{})
	exportedFunctions = append(exportedFunctions,
		&_RegexReplace{})
}
