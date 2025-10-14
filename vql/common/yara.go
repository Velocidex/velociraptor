//go:build cgo && yara
// +build cgo,yara

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
	"www.velocidex.com/golang/velociraptor/accessors"
	"www.velocidex.com/golang/velociraptor/acls"
	"www.velocidex.com/golang/velociraptor/constants"
	"www.velocidex.com/golang/velociraptor/uploads"
	"www.velocidex.com/golang/velociraptor/utils"
	"www.velocidex.com/golang/velociraptor/vql"
	vql_subsystem "www.velocidex.com/golang/velociraptor/vql"
	"www.velocidex.com/golang/velociraptor/vql/functions"
	vfilter "www.velocidex.com/golang/vfilter"
	"www.velocidex.com/golang/vfilter/arg_parser"
	"www.velocidex.com/golang/vfilter/types"
)

type YaraScanPluginArgs struct {
	Rules           string            `vfilter:"optional,field=rules,doc=Yara rules in the yara DSL or after being compiled by the yarac compiler."`
	Files           []types.Any       `vfilter:"required,field=files,doc=The list of files to scan."`
	Accessor        string            `vfilter:"optional,field=accessor,doc=Accessor (e.g. ntfs,file)"`
	Context         int               `vfilter:"optional,field=context,doc=How many bytes to include around each hit"`
	Start           uint64            `vfilter:"optional,field=start,doc=The start offset to scan"`
	End             uint64            `vfilter:"optional,field=end,doc=End scanning at this offset (100mb)"`
	NumberOfHits    int64             `vfilter:"optional,field=number,doc=Stop after this many hits (1)."`
	Blocksize       uint64            `vfilter:"optional,field=blocksize,doc=Blocksize for scanning (1mb)."`
	Key             string            `vfilter:"optional,field=key,doc=If set use this key to cache the  yara rules."`
	Namespace       string            `vfilter:"optional,field=namespace,doc=The Yara namespece to use."`
	YaraVariables   *ordereddict.Dict `vfilter:"optional,field=vars,doc=The Yara variables to use."`
	ForceBufferScan bool              `vfilter:"optional,field=force_buffers,doc=Force buffer scan in all cases."`
}

type YaraScanPlugin struct{}

func (self YaraScanPlugin) Call(
	ctx context.Context,
	scope vfilter.Scope,
	args *ordereddict.Dict) <-chan vfilter.Row {
	output_chan := make(chan vfilter.Row)

	go func() {
		defer close(output_chan)
		defer vql_subsystem.RegisterMonitor(ctx, "yara", args)()

		arg := &YaraScanPluginArgs{}
		err := arg_parser.ExtractArgsWithContext(ctx, scope, args, arg)
		if err != nil {
			scope.Error("yara: %v", err)
			return
		}

		if arg.NumberOfHits == 0 {
			arg.NumberOfHits = 1
		}

		if arg.Blocksize == 0 {
			arg.Blocksize = 1024 * 1024
		}

		rules, err := getYaraRules(arg.Key, arg.Namespace, arg.Rules,
			arg.YaraVariables, scope)
		if err != nil {
			functions.DeduplicatedLog(ctx, scope, "ERROR:yara: "+err.Error())
			return
		}

		yara_flag := yara.ScanFlags(0)
		if arg.NumberOfHits == 1 {
			yara_flag = yara.ScanFlagsFastMode
		}

		logger, closer := utils.NewDeduplicatedLogger(10 * time.Second)
		defer closer()

		matcher := &scanReporter{
			output_chan:    output_chan,
			blocksize:      arg.Blocksize,
			number_of_hits: arg.NumberOfHits,
			context:        arg.Context,
			ctx:            ctx,
			log_level: vql_subsystem.GetIntFromRow(
				scope, scope, constants.YARA_LOG_LEVEL),
			logger:    logger,
			rules:     rules,
			scope:     scope,
			yara_flag: yara_flag,
		}

		accessor, err := accessors.GetAccessor(arg.Accessor, scope)
		if err != nil {
			scope.Error("yara: %v", err)
			return
		}

		for _, filename_any := range arg.Files {
			filename, err := accessors.ParseOSPath(
				ctx, scope, accessor, filename_any)
			if err != nil {
				scope.Log("yara: %v", err)
				return
			}
			matcher.filename = filename

			// As an optimization, we try to call yara's ScanFile API
			// which mmaps the entire file into memory avoiding the
			// need for buffering.
			raw_accessor, ok := accessor.(accessors.RawFileAPIAccessor)

			// If the start offset is specified we always use the
			// accessor.
			if !arg.ForceBufferScan && arg.Start == 0 && ok {
				underlying_file, err := raw_accessor.GetUnderlyingAPIFilename(filename)
				if err == nil {
					err := matcher.scanFile(ctx, underlying_file, output_chan)
					if err == nil {
						continue
					} else {
						scope.Log("yara: Directly scanning file %v failed, will use accessor",
							filename.String())
					}
				}
			}

			// If scanning with the file api failed above
			// we fall back to accessor scanning.
			matcher.scanFileByAccessor(ctx, arg.Accessor, accessor,
				arg.Blocksize, arg.Start, arg.End, output_chan)
		}
	}()

	return output_chan
}

// Yara rules are cached in the scope cache so it is very efficient to
// call the yara plugin repeatadly on the same rules - we do not need
// to recompile the rules all the time. We use the key as the cache or
// the hash of the rules string if not provided.
func getYaraRules(key, namespace, rules string,
	vars *ordereddict.Dict, scope vfilter.Scope) (*yara.Rules, error) {

	// Try to get the compiled yara expression from the
	// scope cache.
	if key == "" {
		// md5sum is good enough for this.
		rule_hash := md5.Sum([]byte(rules))
		key = string(rule_hash[:])
	}
	cached_result := vql_subsystem.CacheGet(scope, key)
	if cached_result != nil {
		switch t := cached_result.(type) {
		case error:
			return nil, t

		case *yara.Rules:
			return t, nil

		default:
			// Unknown type - recompile again.
		}
	}

	compiled_rules, err := compileRules(
		scope, vars, key, namespace, rules)
	if err != nil {
		vql_subsystem.CacheSet(scope, key, err)
		return nil, err
	}

	// Cache the successful rules for further use
	vql_subsystem.CacheSet(scope, key, compiled_rules)
	return compiled_rules, nil
}

func compileRules(scope vfilter.Scope,
	vars *ordereddict.Dict,
	key, namespace, rules string) (*yara.Rules, error) {

	// Might be a compiled ruleset.
	if strings.HasPrefix(rules, "YARA") {
		return yara.ReadRules(strings.NewReader(rules))
	}

	generated_rules := RuleGenerator(scope, rules)
	compiler, err := yara.NewCompiler()
	if err != nil {
		return nil, err
	}

	if vars != nil {
		for _, i := range vars.Items() {
			err := compiler.DefineVariable(i.Key, i.Value)
			if err != nil {
				vql_subsystem.CacheSet(scope, i.Key, err)
				return nil, err
			}
		}
	}

	err = compiler.AddString(generated_rules, namespace)
	if err != nil {
		// Cache the compile failure so only one log is emitted.
		vql_subsystem.CacheSet(scope, key, err)
		return nil, err
	}

	return compiler.GetRules()

}

func (self *scanReporter) scanFileByAccessor(
	ctx context.Context,
	accessor_name string,
	accessor accessors.FileSystemAccessor,
	blocksize uint64,
	start, end uint64,
	output_chan chan vfilter.Row) {

	defer utils.CheckForPanic("Panic in scanFileByAccessor")

	// Open the file with the accessor
	f, err := accessor.OpenWithOSPath(self.filename)
	if err != nil {
		self.scope.Log("yara: Failed to open %v with accessor %v: %v",
			self.filename, accessor_name, err)
		return
	}
	defer f.Close()

	self.file_info, err = accessor.LstatWithOSPath(self.filename)
	if err != nil {
		self.scope.Log("yara: Failed to open %v with accessor %v: %v",
			self.filename, accessor_name, err)
		return
	}
	self.reader = utils.MakeReaderAtter(f)

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

func (self *scanReporter) scanRange(start, end uint64, f accessors.ReadSeekCloser) {
	buf := make([]byte, self.blocksize)

	if self.log_level >= 1 {
		self.logger.Log(self.scope,
			"Scanning %v from %#0x to %#0x", self.filename, start, end)
	}

	// base_offset reflects the file offset where we scan.
	for self.base_offset = start; self.base_offset < end; {
		// Try to seek to the start offset - if it does not work then
		// dont worry about it - just start from the beginning. This
		// is needed for scanning devices which may not advance their
		// own file pointer when read so we force a seek on each read.
		_, _ = f.Seek(int64(self.base_offset), 0)

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

		scanner, err := yara.NewScanner(self.rules)
		if err != nil {
			return
		}

		// There is no way to actively cancel the yara scan so this is
		// as good as we can get - do not set the timeout too long or
		// we wont be able to cancel it promptly.
		err = scanner.SetCallback(self).
			SetTimeout(100 * time.Second).
			SetFlags(self.yara_flag).
			ScanMem(scan_buf)
		if err != nil {
			return
		}

		// Advance the read pointer
		self.base_offset += uint64(n)
		self.reader = nil

		if self.log_level >= 2 {
			self.logger.Log(self.scope,
				"Range %v from %#0x to %#0x: Got to %#0x (%d %%)",
				self.filename, start, end, self.base_offset,
				100*(self.base_offset-start)/(end-start))
		}

		// We count an op as one MB scanned.
		self.scope.ChargeOp()
	}
}

// Scan a file called when no accessor is specified. We pass the
// filename to libyara directly for faster scanning using mmap. This
// also ensures that all yara features (like the PE plugin) work.
func (self *scanReporter) scanFile(
	ctx context.Context,
	underlying_file string,
	output_chan chan vfilter.Row) error {

	fd, err := os.Open(underlying_file)
	if err != nil {
		return err
	}
	defer fd.Close()

	// Fill in the file stat if possible.
	file_accessor, err := accessors.GetAccessor("auto", self.scope)
	if err == nil {
		self.file_info, _ = file_accessor.LstatWithOSPath(self.filename)
	}
	self.reader = fd

	scanner, err := yara.NewScanner(self.rules)
	if err != nil {
		return err
	}

	// There is no way to actively cancel the yara scan so this is as
	// good as we can get - do not set the timeout too long or we wont
	// be able to cancel it promptly.
	err = scanner.SetCallback(self).
		SetTimeout(100 * time.Second).
		SetFlags(self.yara_flag).
		ScanFile(underlying_file)
	if err != nil {
		return err
	}

	// We count an op as one MB scanned.
	self.scope.ChargeOp()

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
	file_info      accessors.FileInfo
	filename       *accessors.OSPath
	base_offset    uint64
	end            uint64
	reader         io.ReaderAt
	ctx            context.Context
	log_level      uint64
	logger         *utils.DeduplicatedLogger

	// Internal scan state
	scope     vfilter.Scope
	rules     *yara.Rules
	yara_flag yara.ScanFlags
}

func (self *scanReporter) getMeta(rule *yara.Rule) *ordereddict.Dict {
	metas := rule.Metas()
	if len(metas) > 0 {
		result := ordereddict.NewDict()
		for _, m := range metas {
			result.Set(m.Identifier, m.Value)
		}
		return result
	}
	return nil
}

func (self *scanReporter) RuleMatching(
	scan_context *yara.ScanContext, rule *yara.Rule) (bool, error) {
	matches := getMatchStrings(scan_context, rule)
	metas := self.getMeta(rule)

	// The rule matched no strings, just emit a single row.
	if len(matches) == 0 {
		res := &YaraResult{
			Rule:     rule.Identifier(),
			Tags:     rule.Tags(),
			Meta:     metas,
			File:     self.file_info,
			FileName: self.filename,

			// There are no strings so produce an empty String member.
			String: &YaraHit{},
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
		n, _ := self.reader.ReadAt(data, int64(context_start)+int64(match_string.Base))
		data = data[:n]

		res := &YaraResult{
			Rule:     rule.Identifier(),
			Tags:     rule.Tags(),
			Meta:     metas,
			File:     self.file_info,
			FileName: self.filename,
			String: &YaraHit{
				Name:    match_string.Name,
				Offset:  match_string.Offset + self.base_offset + match_string.Base,
				Data:    data,
				HexData: strings.Split(hex.Dump(data), "\n"),
			},
		}
		if self.end > 0 && res.String.Offset >= self.end {
			return false, nil
		}

		// Emit the results.
		select {
		case <-self.ctx.Done():
			return false, nil
		case self.output_chan <- res:
		}

		self.number_of_hits--
		if self.number_of_hits <= 0 {
			return false, nil
		}

	}

	return true, nil
}

func getMatchStrings(scan_context *yara.ScanContext, r *yara.Rule) (
	matchstrings []yara.MatchString) {
	for _, s := range r.Strings() {
		for _, m := range s.Matches(scan_context) {
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
		Name:     "yara",
		Doc:      "Scan files using yara rules.",
		ArgType:  type_map.AddType(scope, &YaraScanPluginArgs{}),
		Metadata: vql.VQLMetadata().Permissions(acls.FILESYSTEM_READ).Build(),
	}
}

type YaraProcPluginArgs struct {
	Rules         string            `vfilter:"required,field=rules,doc=Yara rules"`
	Pid           int               `vfilter:"required,field=pid,doc=The pid to scan"`
	Context       int               `vfilter:"optional,field=context,doc=Return this many bytes either side of a hit"`
	Key           string            `vfilter:"optional,field=key,doc=If set use this key to cache the  yara rules."`
	Namespace     string            `vfilter:"optional,field=namespace,doc=The Yara namespece to use."`
	YaraVariables *ordereddict.Dict `vfilter:"optional,field=vars,doc=The Yara variables to use."`
	NumberOfHits  int64             `vfilter:"optional,field=number,doc=Stop after this many hits (1)."`
}

type YaraProcPlugin struct{}

func (self YaraProcPlugin) Info(
	scope vfilter.Scope,
	type_map *vfilter.TypeMap) *vfilter.PluginInfo {
	return &vfilter.PluginInfo{
		Name:     "proc_yara",
		Doc:      "Scan processes using yara rules.",
		ArgType:  type_map.AddType(scope, &YaraProcPluginArgs{}),
		Metadata: vql.VQLMetadata().Permissions(acls.MACHINE_STATE).Build(),
	}
}

func (self YaraProcPlugin) Call(
	ctx context.Context,
	scope vfilter.Scope,
	args *ordereddict.Dict) <-chan vfilter.Row {
	output_chan := make(chan vfilter.Row)

	go func() {
		defer close(output_chan)
		defer vql_subsystem.RegisterMonitor(ctx, "proc_yara", args)()

		arg := &YaraProcPluginArgs{}
		err := arg_parser.ExtractArgsWithContext(ctx, scope, args, arg)
		if err != nil {
			scope.Log("proc_yara: %v", err)
			return
		}

		err = vql_subsystem.CheckAccess(scope, acls.MACHINE_STATE)
		if err != nil {
			scope.Log("proc_yara: %v", err)
			return
		}

		accessor, err := accessors.GetAccessor("process", scope)
		if err != nil {
			scope.Log("proc_yara: %v", err)
			return
		}

		filename := fmt.Sprintf("/%v", arg.Pid)
		process_stat, err := accessor.Lstat(filename)
		if err != nil {
			scope.Log("proc_yara: %v", err)
			return
		}

		// Open a handle into the process so we can read context out
		process_address_space, err := accessor.OpenWithOSPath(
			process_stat.OSPath())
		if err != nil {
			scope.Log("proc_yara: %v", err)
			return
		}
		defer process_address_space.Close()

		rules, err := getYaraRules(arg.Key, arg.Namespace,
			arg.Rules, arg.YaraVariables, scope)
		if err != nil {
			functions.DeduplicatedLog(ctx, scope, "ERROR:proc_yara: "+err.Error())
			return
		}

		scanner, err := yara.NewScanner(rules)
		if err != nil {
			functions.DeduplicatedLog(ctx, scope, "ERROR:proc_yara: "+err.Error())
			return
		}

		yara_flag := yara.ScanFlags(0)
		if arg.NumberOfHits == 1 {
			yara_flag = yara.ScanFlagsFastMode
		}

		matcher := &scanReporter{
			output_chan:    output_chan,
			number_of_hits: arg.NumberOfHits,
			context:        arg.Context,
			ctx:            ctx,
			reader:         utils.MakeReaderAtter(process_address_space),

			rules:     rules,
			scope:     scope,
			filename:  process_stat.OSPath(),
			file_info: process_stat,
			yara_flag: yara_flag,
		}

		// There is no way to actively cancel the yara scan so this is
		// as good as we can get - do not set the timeout too long or
		// we wont be able to cancel it promptly.
		err = scanner.SetCallback(matcher).
			SetTimeout(100 * time.Second).
			SetFlags(yara_flag).
			ScanProc(arg.Pid)
		if err != nil {
			scope.Log("proc_yara: pid %v: %v", arg.Pid, err)
			return
		}

		scope.ChargeOp()
	}()

	return output_chan
}

// Provide a shortcut way to define common rules.
func RuleGenerator(scope vfilter.Scope, rule string) string {
	rule = strings.TrimSpace(rule)

	// Just a normal yara rule
	if strings.HasPrefix(rule, "rule") ||
		strings.HasPrefix(rule, "import") {
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
			scope.Log("yara: Warning unknown shorthand directive %v - treating as Yara Rule", kw)
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
