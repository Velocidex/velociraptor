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
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	yara "github.com/Velocidex/go-yara"
	"github.com/Velocidex/ordereddict"
	"www.velocidex.com/golang/velociraptor/glob"
	"www.velocidex.com/golang/velociraptor/uploads"
	"www.velocidex.com/golang/velociraptor/utils"
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
	Blocksize    uint64   `vfilter:"optional,field=blocksize,doc=Blocksize for scanning (1mb)."`
	Key          string   `vfilter:"optional,field=key,doc=If set use this key to cache the  yara rules."`
}

type YaraScanPlugin struct{}

func (self YaraScanPlugin) Call(
	ctx context.Context,
	scope vfilter.Scope,
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
			return
		}

		yara_flag := yara.ScanFlags(0)
		if arg.NumberOfHits == 1 {
			yara_flag = yara.ScanFlagsFastMode
		}

		matcher := &scanReporter{
			output_chan:    output_chan,
			blocksize:      arg.Blocksize,
			number_of_hits: arg.NumberOfHits,
			context:        arg.Context,
			ctx:            ctx,

			rules:     rules,
			scope:     scope,
			yara_flag: yara_flag,
		}

		for _, filename := range arg.Files {

			matcher.filename = filename

			// If accessor is not specified we call yara's
			// ScanFile API which mmaps the entire file
			// into memory avoiding the need for
			// buffering.
			if arg.Accessor == "" || arg.Accessor == "file" {
				err := matcher.scanFile(ctx, output_chan)
				if err == nil {
					continue
				} else {
					scope.Log("Directly scanning file %v failed, will use accessor",
						filename)
				}
			}

			// If scanning with the file api failed above
			// we fall back to accessor scanning.
			matcher.scanFileByAccessor(ctx, arg.Accessor,
				arg.Blocksize, arg.Start, arg.End, output_chan)
		}
	}()

	return output_chan
}

// Yara rules are cached in the scope cache so it is very efficient to
// call the yara plugin repeatadly on the same rules - we do not need
// to recompile the rules all the time. We use the key as the cache or
// the hash of the rules string if not provided.
func getYaraRules(key, rules string,
	scope vfilter.Scope) (*yara.Rules, error) {

	// Try to get the compiled yara expression from the
	// scope cache.
	if key == "" {
		// md5sum is good enough for this.
		rule_hash := md5.Sum([]byte(rules))
		key = string(rule_hash[:])
	}
	result := vql_subsystem.CacheGet(scope, key)
	if result == nil {
		variables := make(map[string]interface{})
		generated_rules := RuleGenerator(scope, rules)
		result, err := yara.Compile(generated_rules, variables)
		if err != nil {
			// Cache the compile failure so only one log is emitted.
			scope.Log("Failed to initialize YARA compiler: %s", err)
			vql_subsystem.CacheSet(scope, key, err)
			return nil, err
		}
		vql_subsystem.CacheSet(scope, key, result)
		return result, nil
	}

	switch t := result.(type) {
	case error:
		return nil, t
	case *yara.Rules:
		return t, nil
	default:
		return nil, errors.New("Error")
	}
}

func (self *scanReporter) scanFileByAccessor(
	ctx context.Context,
	accessor_name string,
	blocksize uint64,
	start, end uint64,
	output_chan chan vfilter.Row) {

	accessor, err := glob.GetAccessor(accessor_name, self.scope)
	if err != nil {
		self.scope.Log("yara: %v", err)
		return
	}

	// Open the file with the accessor
	f, err := accessor.Open(self.filename)
	if err != nil {
		self.scope.Log("Failed to open %v", self.filename)
		return
	}
	defer f.Close()

	self.file_info, _ = f.Stat()
	self.reader = utils.ReaderAtter{f}

	// Support sparse file scanning
	range_reader, ok := f.(uploads.RangeReader)
	if !ok {
		// File does not support ranges, just cap the end at
		// the end of the file.
		if end == 0 && self.file_info != nil {
			end = uint64(self.file_info.Size())
		}

		self.scanRange(start, end, f)
		return
	}

	for _, rng := range range_reader.Ranges() {
		if !rng.IsSparse {
			scan_start := uint64(rng.Offset)
			if scan_start < start {
				scan_start = start
			}

			scan_end := uint64(rng.Offset + rng.Length)
			if end > 0 && scan_end > end {
				scan_end = end
			}

			if scan_start > scan_end {
				continue
			}

			self.scanRange(scan_start, scan_end, f)
		}
	}
}

func (self *scanReporter) scanRange(start, end uint64, f glob.ReadSeekCloser) {
	// Try to seek to the start offset - if it does not work then
	// dont worry about it - just start from the beginning.
	_, _ = f.Seek(int64(start), 0)
	buf := make([]byte, self.blocksize)

	self.scope.Log("Scanning %v from %#0x to %#0x", self.filename, start, end)

	// base_offset reflects the file offset where we scan.
	for self.base_offset = start; self.base_offset < end; {
		// Only read up to the end of the range
		to_read := end - start
		if to_read > self.blocksize {
			to_read = self.blocksize
		}

		n, _ := f.Read(buf[:to_read])
		if n == 0 {
			return
		}

		scan_buf := buf[:n]

		// Set the reader and base_offset before we call the
		// yara callback so it can report the correct offset
		// match and extract any context data.
		self.reader = bytes.NewReader(scan_buf)

		err := self.rules.ScanMemWithCallback(
			scan_buf, self.yara_flag, 10*time.Second, self)
		if err != nil {
			return
		}

		// Advance the read pointer
		self.base_offset += uint64(n)

		// We count an op as one MB scanned.
		vfilter.ChargeOp(self.scope)
	}
}

// Scan a file called when no accessor is specified. We pass the
// filename to libyara directly for faster scanning using mmap. This
// also ensures that all yara features (like the PE plugin) work.
func (self *scanReporter) scanFile(
	ctx context.Context, output_chan chan vfilter.Row) error {

	fd, err := os.Open(self.filename)
	if err != nil {
		return err
	}
	defer fd.Close()

	// Fill in the file stat if possible.
	self.file_info, _ = fd.Stat()
	self.reader = fd

	err = self.rules.ScanFileWithCallback(
		self.filename, self.yara_flag, 10*time.Second, self)
	if err != nil {
		return err
	}

	// We count an op as one MB scanned.
	vfilter.ChargeOp(self.scope)

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
	blocksize      uint64
	context        int
	file_info      os.FileInfo
	filename       string
	base_offset    uint64
	end            uint64
	reader         io.ReaderAt
	ctx            context.Context

	// Internal scan state
	scope     vfilter.Scope
	rules     *yara.Rules
	yara_flag yara.ScanFlags
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
		select {
		case <-self.ctx.Done():
			return false, nil

		case self.output_chan <- res:
		}
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
	scope vfilter.Scope,
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
	scope vfilter.Scope,
	type_map *vfilter.TypeMap) *vfilter.PluginInfo {
	return &vfilter.PluginInfo{
		Name:    "proc_yara",
		Doc:     "Scan processes using yara rules.",
		ArgType: type_map.AddType(scope, &YaraProcPluginArgs{}),
	}
}

func (self YaraProcPlugin) Call(
	ctx context.Context,
	scope vfilter.Scope,
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
			scope.Log("proc_yara: pid %v: %v", arg.Pid, err)
			return
		}

		for _, match := range matches {
			select {
			case <-ctx.Done():
				return

			case output_chan <- match:
			}
		}

		vfilter.ChargeOp(scope)
	}()

	return output_chan
}

// Provide a shortcut way to define common rules.
func RuleGenerator(scope vfilter.Scope, rule string) string {
	rule = strings.TrimSpace(rule)

	// Just a normal yara rule
	if strings.HasPrefix(rule, "rule") {
		return rule
	}

	// Shorthand syntax looks like:
	// ascii wide: foo,bar,baz

	tmp := strings.SplitN(rule, ":", 2)
	if len(tmp) != 2 {
		return rule
	}
	keywords, data := tmp[0], tmp[1]
	data = strings.TrimSpace(data)

	method := ""
	for _, kw := range strings.Split(keywords, " ") {
		switch kw {
		case "wide", "ascii", "nocase":
			method += " " + kw
		default:
			scope.Log("Unknown shorthand directive %v", kw)
			return rule
		}
	}

	string_clause := ""
	for idx, item := range strings.Split(data, ",") {
		item = strings.TrimSpace(item)
		string_clause += fmt.Sprintf(
			" $a%v = \"%s\" %s\n", idx, item, method)
		scope.Log("Compiling shorthand yara rule %v", string_clause)
	}

	return fmt.Sprintf(
		"rule X { strings: %s condition: any of them }",
		string_clause)
}

func init() {
	vql_subsystem.RegisterPlugin(&YaraScanPlugin{})
	vql_subsystem.RegisterPlugin(&YaraProcPlugin{})
}
