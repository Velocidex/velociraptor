package vql

import (
	"bufio"
	"fmt"
	"os"
	_ "regexp"
	utils "www.velocidex.com/golang/velociraptor/testing"
	"www.velocidex.com/golang/vfilter"
)

func _ParseFileWithRegex(args *vfilter.Dict) []vfilter.Row {
	var result []vfilter.Row
	filename, ok := vfilter.ExtractString("file", args)
	if !ok {
		return result
	}

	utils.Debug(filename)
	regexps, ok := vfilter.ExtractStringArray("regex", args)
	if !ok {
		return result
	}

	utils.Debug(regexps)

	file, err := os.Open(*filename)
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

func MakeRegexParserPlugin() vfilter.GenericListPlugin {
	return vfilter.GenericListPlugin{
		PluginName: "parse_with_regex",
		Function:   _ParseFileWithRegex,
		RowType:    vfilter.Dict{},
	}
}
