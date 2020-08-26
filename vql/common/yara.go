// +build cgo,yara

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

package common

import (
	"bytes"
	"context"
	"crypto/md5"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	yara "github.com/Velocidex/go-yara"
	"github.com/Velocidex/ordereddict"
	"www.velocidex.com/golang/velociraptor/glob"
	vql_subsystem "www.velocidex.com/golang/velociraptor/vql"
	vfilter "www.velocidex.com/golang/vfilter"
)

type YaraHit struct {
	Name    string
	Offset  uint64
	HexData []string
	Data    []byte
}

type YaraResult struct {
	Rule     string
	Meta     map[string]interface{}
	Tags     []string
	String   *YaraHit
	File     os.FileInfo
	FileName string
}

type YaraScanPluginArgs struct {
	Rules        string   `vfilter:"required,field=rules,doc=Yara rules in the yara DSL."`
	Files        []string `vfilter:"required,field=files,doc=The list of files to scan."`
	Accessor     string   `vfilter:"optional,field=accessor,doc=Accessor (e.g. NTFS)"`
	Context      int      `vfilter:"optional,field=context,doc=How many bytes to include around each hit"`
	Start        uint64   `vfilter:"optional,field=start,doc=The start offset to scan"`
	End          uint64   `vfilter:"optional,field=end,doc=End scanning at this offset (100mb)"`
	NumberOfHits int64    `vfilter:"optional,field=number,doc=Stop after this many hits (1)."`
	Blocksize    int64    `vfilter:"optional,field=blocksize,doc=Blocksize for scanning (1mb)."`
	Key          string   `vfilter:"optional,field=key,doc=If set use this key to cache the  yara rules."`
}

type YaraScanPlugin struct{}

func (self YaraScanPlugin) Call(
	ctx context.Context,
	scope *vfilter.Scope,
	args *ordereddict.Dict) <-chan vfilter.Row {
	output_chan := make(chan vfilter.Row)

	go func() {
		defer close(output_chan)

		arg := &YaraScanPluginArgs{}
		err := vfilter.ExtractArgs(scope, args, arg)
		if err != nil {
			scope.Log("yarascan: %v", err)
			return
		}

		if arg.End == 0 {
			arg.End = 1024 * 1024 * 100
		}

		if arg.NumberOfHits == 0 {
			arg.NumberOfHits = 1
		}

		if arg.Blocksize == 0 {
			arg.Blocksize = 1024 * 1024
		}

		err = vql_subsystem.CheckFilesystemAccess(scope, arg.Accessor)
		if err != nil {
			scope.Log("yara: %s", err.Error())
			return
		}

		rules, err := getYaraRules(arg.Key, arg.Rules, scope)
		if err != nil {
			scope.Log("Failed to initialize YARA compiler: %s", err)
			return
		}

		for _, filename := range arg.Files {
			// If accessor is not specified we call yara's
			// ScanFile API which mmaps the entire file
			// into memory avoiding the need for
			// buffering.
			if arg.Accessor == "" {
				err := scanFile(filename, arg.Context, arg.NumberOfHits,
					rules, output_chan, scope)

				// Fall back to accessor scanning if
				// we can not open the file directly.
				if err == nil {
					continue
				}
			}

			scanFileByAccessor(filename, arg.Accessor,
				arg.Blocksize, arg.Start, arg.End,
				arg.Context, arg.NumberOfHits,
				rules, output_chan, scope)
		}
	}()

	return output_chan
}

// Yara rules are cached in the scope cache so it is very efficient to
// call the yara plugin repeatadly on the same rules - we do not need
// to recompile the rules all the time. We use the key as the cache or
// the hash of the rules string if not provided.
func getYaraRules(key, rules string,
	scope *vfilter.Scope) (*yara.Rules, error) {
	var err error

	// Try to get the compiled yara expression from the
	// scope cache.
	if key == "" {
		// md5sum is good enough for this.
		rule_hash := md5.Sum([]byte(rules))
		key = string(rule_hash[:])
	}
	result, ok := vql_subsystem.CacheGet(scope, key).(*yara.Rules)
	if !ok {
		variables := make(map[string]interface{})
		generated_rules := RuleGenerator(scope, rules)
		result, err = yara.Compile(generated_rules, variables)
		if err != nil {
			return nil, err
		}
		vql_subsystem.CacheSet(scope, key, result)
	}

	return result, nil
}

func scanFileByAccessor(
	filename, accessor_name string,
	blocksize int64,
	start, end uint64,
	context int,
	total_number_of_hits int64,
	rules *yara.Rules,
	output_chan chan vfilter.Row,
	scope *vfilter.Scope) {

	accessor, err := glob.GetAccessor(accessor_name, scope)
	if err != nil {
		scope.Log("yara: %v", err)
		return
	}

	yara_flag := yara.ScanFlags(0)
	if total_number_of_hits == 1 {
		yara_flag = yara.ScanFlagsFastMode
	}

	// Total hits per file.
	f, err := accessor.Open(filename)
	if err != nil {
		scope.Log("Failed to open %v", filename)
		return
	}
	defer f.Close()

	// Try to seek to the start offset - if it does not work then
	// dont worry about it - just start from the beginning.
	_, _ = f.Seek(int64(start), 0)

	buf := make([]byte, blocksize)

	stat, _ := f.Stat()

	matcher := &scanReporter{
		output_chan:    output_chan,
		number_of_hits: total_number_of_hits,
		end:            end,
		context:        context,
		file_info:      stat,
		filename:       filename,
		base_offset:    start,
	}

	for {
		n, _ := f.Read(buf)
		if n == 0 {
			return
		}

		scan_buf := buf[:n]
		matcher.reader = bytes.NewReader(scan_buf)

		err := rules.ScanMemWithCallback(
			scan_buf, yara_flag, 10*time.Second, matcher)
		if err != nil {
			return
		}

		matcher.base_offset += uint64(n)
		if matcher.base_offset > uint64(end) {
			return
		}

		// We count an op as one MB scanned.
		vfilter.ChargeOp(scope)
	}
}

func scanFile(
	filename string,
	context int,
	total_number_of_hits int64,
	rules *yara.Rules,
	output_chan chan vfilter.Row,
	scope *vfilter.Scope) error {

	yara_flag := yara.ScanFlags(0)
	if total_number_of_hits == 1 {
		yara_flag = yara.ScanFlagsFastMode
	}

	fd, err := os.Open(filename)
	if err != nil {
		return err
	}
	defer fd.Close()

	stat, _ := fd.Stat()

	matcher := &scanReporter{
		output_chan:    output_chan,
		number_of_hits: total_number_of_hits,
		context:        context,
		file_info:      stat,
		filename:       filename,
		reader:         fd,
	}

	err = rules.ScanFileWithCallback(
		filename, yara_flag, 10*time.Second, matcher)
	if err != nil {
		return err
	}

	// We count an op as one MB scanned.
	vfilter.ChargeOp(scope)

	return nil
}

// Reports all hits in the match and includes any required context. We
// report one row per matching string in the signature, unless no
// strings match in which case we report a single row for the
// match. The number_of_hits parameter specified how many hits the
// caller is interested in so we can breakout early. This returns the
// total number of hits actually reported.
type scanReporter struct {
	output_chan    chan vfilter.Row
	number_of_hits int64
	context        int
	file_info      os.FileInfo
	filename       string
	base_offset    uint64
	end            uint64
	reader         io.ReaderAt
}

func (self *scanReporter) RuleMatching(rule *yara.Rule) (bool, error) {
	matches := getMatchStrings(rule)

	// The rule matched no strings, just emit a single row.
	if len(matches) == 0 {
		res := &YaraResult{
			Rule:     rule.Identifier(),
			Tags:     rule.Tags(),
			Meta:     rule.Metas(),
			File:     self.file_info,
			FileName: self.filename,
		}
		self.output_chan <- res
		self.number_of_hits--
		if self.number_of_hits <= 0 {
			return false, nil
		}

		return true, nil
	}

	// If the rule matches multiple strings, emit one string per row.
	for _, match_string := range matches {

		// Extract a larger context from the scan buffer.
		context_start := int(match_string.Offset) - self.context
		if context_start < 0 {
			context_start = 0
		}

		context_end := int(match_string.Offset) + len(match_string.Data) + self.context

		// Make a copy of the underlying data.
		data := make([]byte, context_end-context_start)
		n, _ := self.reader.ReadAt(data, int64(context_start))
		data = data[:n]

		res := &YaraResult{
			Rule:     rule.Identifier(),
			Tags:     rule.Tags(),
			Meta:     rule.Metas(),
			File:     self.file_info,
			FileName: self.filename,
			String: &YaraHit{
				Name:    match_string.Name,
				Offset:  match_string.Offset + self.base_offset,
				Data:    data,
				HexData: strings.Split(hex.Dump(data), "\n"),
			},
		}
		if self.end > 0 && res.String.Offset >= self.end {
			return false, nil
		}

		// Emit the results.
		self.output_chan <- res

		self.number_of_hits--
		if self.number_of_hits <= 0 {
			return false, nil
		}

	}

	return true, nil
}

func getMatchStrings(r *yara.Rule) (matchstrings []yara.MatchString) {
	for _, s := range r.Strings() {
		for _, m := range s.Matches() {
			matchstrings = append(matchstrings, yara.MatchString{
				Name:   s.Identifier(),
				Base:   uint64(m.Base()),
				Offset: uint64(m.Offset()),
				Data:   m.Data(),
			})
		}
	}
	return
}

func (self YaraScanPlugin) Info(
	scope *vfilter.Scope,
	type_map *vfilter.TypeMap) *vfilter.PluginInfo {
	return &vfilter.PluginInfo{
		Name:    "yara",
		Doc:     "Scan files using yara rules.",
		ArgType: type_map.AddType(scope, &YaraScanPluginArgs{}),
	}
}

type YaraProcPluginArgs struct {
	Rules   string `vfilter:"required,field=rules,doc=Yara rules"`
	Pid     int    `vfilter:"required,field=pid,doc=The pid to scan"`
	Context int    `vfilter:"optional,field=context,doc=Return this many bytes either side of a hit"`
	Key     string `vfilter:"optional,field=key,doc=If set use this key to cache the  yara rules."`
}

type YaraProcPlugin struct{}

func (self YaraProcPlugin) Info(
	scope *vfilter.Scope,
	type_map *vfilter.TypeMap) *vfilter.PluginInfo {
	return &vfilter.PluginInfo{
		Name:    "proc_yara",
		Doc:     "Scan processes using yara rules.",
		ArgType: type_map.AddType(scope, &YaraScanPluginArgs{}),
	}
}

func (self YaraProcPlugin) Call(
	ctx context.Context,
	scope *vfilter.Scope,
	args *ordereddict.Dict) <-chan vfilter.Row {
	output_chan := make(chan vfilter.Row)

	go func() {
		defer close(output_chan)

		arg := &YaraProcPluginArgs{}
		err := vfilter.ExtractArgs(scope, args, arg)
		if err != nil {
			scope.Log("proc_yara: %v", err)
			return
		}

		if arg.Key == "" {
			rule_hash := md5.Sum([]byte(arg.Rules))
			arg.Key = string(rule_hash[:])
		}
		rules, ok := vql_subsystem.CacheGet(
			scope, arg.Key).(*yara.Rules)
		if !ok {
			variables := make(map[string]interface{})
			generated_rules := RuleGenerator(scope, arg.Rules)
			rules, err = yara.Compile(generated_rules, variables)
			if err != nil {
				scope.Log("Failed to initialize YARA compiler: %v", err)
				return
			}

			vql_subsystem.CacheSet(scope, arg.Key, rules)
		}

		matches, err := rules.ScanProc(
			arg.Pid, yara.ScanFlagsProcessMemory,
			300*time.Second)
		if err != nil {
			scope.Log("Failed: %v", err)
			return
		}

		for _, match := range matches {
			output_chan <- match
		}

		vfilter.ChargeOp(scope)
	}()

	return output_chan
}

// Provide a shortcut way to define common rules.
func RuleGenerator(scope *vfilter.Scope, rule string) string {
	rule = strings.TrimSpace(rule)

	// Just a normal yara rule
	if strings.HasPrefix(rule, "rule") {
		return rule
	}

	tmp := strings.SplitN(rule, ":", 2)
	if len(tmp) != 2 {
		return rule
	}
	method, data := tmp[0], tmp[1]
	switch method {
	case "wide", "wide ascii", "wide nocase", "wide nocase ascii":
		string_clause := ""
		for _, item := range strings.Split(data, ",") {
			item = strings.TrimSpace(item)
			string_clause += fmt.Sprintf(
				" $ = \"%s\" %s\n", item, method)
			scope.Log("Compiling shorthand yara rule %v", string_clause)
		}

		return fmt.Sprintf(
			"rule X { strings: %s condition: any of them }",
			string_clause)
	}

	return rule
}

func init() {
	vql_subsystem.RegisterPlugin(&YaraScanPlugin{})
	vql_subsystem.RegisterPlugin(&YaraProcPlugin{})
}
