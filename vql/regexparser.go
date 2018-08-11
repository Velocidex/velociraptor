package vql

import (
	"bufio"
	"fmt"
	_ "regexp"
	"www.velocidex.com/golang/velociraptor/glob"
	utils "www.velocidex.com/golang/velociraptor/testing"
	"www.velocidex.com/golang/vfilter"
)

func _ParseFileWithRegex(
	scope *vfilter.Scope,
	args *vfilter.Dict) []vfilter.Row {
	var result []vfilter.Row
	filename, ok := vfilter.ExtractString("file", args)
	if !ok {
		return result
	}

	utils.Debug(filename)
	regexps, ok := vfilter.ExtractStringArray(scope, "regex", args)
	if !ok {
		return result
	}

	utils.Debug(regexps)

	accessor := glob.OSFileSystemAccessor{}
	file, err := accessor.Open(*filename)
	if err != nil {
		return result
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		fmt.Println(scanner.Text())
	}

	return result
}

func init() {
	exportedPlugins = append(exportedPlugins,
		vfilter.GenericListPlugin{
			PluginName: "parse_with_regex",
			Function:   _ParseFileWithRegex,
			RowType:    vfilter.Dict{},
		})
}
