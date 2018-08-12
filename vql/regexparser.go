package vql

import (
	"context"
	"fmt"
	"regexp"
	"www.velocidex.com/golang/velociraptor/glob"
	//	debug "www.velocidex.com/golang/velociraptor/testing"
	"www.velocidex.com/golang/vfilter"
)

type _ParseFileWithRegex struct{}

func (self _ParseFileWithRegex) Call(
	ctx context.Context,
	scope *vfilter.Scope,
	args *vfilter.Dict) <-chan vfilter.Row {
	output_chan := make(chan vfilter.Row)

	filename, ok := vfilter.ExtractString("file", args)
	if !ok {
		scope.Log("Expecting string as 'file' parameter")
		close(output_chan)
		return output_chan
	}

	regexps, ok := vfilter.ExtractStringArray(scope, "regex", args)
	if !ok {
		scope.Log("Expecting string list as 'regex' parameter")
		close(output_chan)
		return output_chan
	}

	var compiled_regexs []*regexp.Regexp
	for _, regex := range regexps {
		r, err := regexp.Compile(regex)
		if err != nil {
			scope.Log("Unable to compile regex %s", regex)
			close(output_chan)
			return output_chan
		}
		compiled_regexs = append(compiled_regexs, r)
	}

	accessor := glob.OSFileSystemAccessor{}
	file, err := accessor.Open(*filename)
	if err != nil {
		scope.Log("Unable to open file %s", *filename)
		close(output_chan)
		return output_chan
	}

	go func() {
		defer close(output_chan)
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
						row := vfilter.NewDict()
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
	}()

	return output_chan
}

func (self _ParseFileWithRegex) Name() string {
	return "parse_with_regex"
}

func (self _ParseFileWithRegex) Info(type_map *vfilter.TypeMap) *vfilter.PluginInfo {
	return &vfilter.PluginInfo{
		Name:    "parse_with_regex",
		Doc:     "Parses a file with a set of regexp and yields matches",
		RowType: type_map.AddType(vfilter.NewDict()),
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
	for _, regex := range regexes {
		r, err := regexp.Compile(regex)
		if err != nil {
			scope.Log("Unable to compile regex %s", regex)
			return vfilter.Null{}
		}

		match := r.FindAllStringSubmatch(*data, -1)
		if match != nil {
			names := r.SubexpNames()
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
	return row
}

func (self _ParseStringWithRegexFunction) Name() string {
	return "parse_string_with_regex"
}

func init() {
	exportedPlugins = append(exportedPlugins, &_ParseFileWithRegex{})
	exportedFunctions = append(exportedFunctions,
		&_ParseStringWithRegexFunction{})
}
