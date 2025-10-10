//go:build cgo && amd64 && (windows || linux)
// +build cgo
// +build amd64
// +build windows linux

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
	"context"
	"crypto/md5"
	"encoding/hex"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/Velocidex/ordereddict"
	yara_x "github.com/Velocidex/yara-x-go"
	"www.velocidex.com/golang/velociraptor/accessors"
	"www.velocidex.com/golang/velociraptor/acls"
	"www.velocidex.com/golang/velociraptor/uploads"
	"www.velocidex.com/golang/velociraptor/utils"
	"www.velocidex.com/golang/velociraptor/vql"
	vql_subsystem "www.velocidex.com/golang/velociraptor/vql"
	"www.velocidex.com/golang/velociraptor/vql/functions"
	vfilter "www.velocidex.com/golang/vfilter"
	"www.velocidex.com/golang/vfilter/arg_parser"
	"www.velocidex.com/golang/vfilter/types"
)

type YaraXScanPluginArgs struct {
	Rules           string            `vfilter:"optional,field=rules,doc=Yara rules in the yara DSL or after being compiled by the yarac compiler."`
	Files           []types.Any       `vfilter:"required,field=files,doc=The list of files to scan."`
	Accessor        string            `vfilter:"optional,field=accessor,doc=Accessor (e.g. ntfs,file)"`
	Context         int               `vfilter:"optional,field=context,doc=How many bytes to include around each hit"`
	Start           uint64            `vfilter:"optional,field=start,doc=The start offset to scan"`
	End             uint64            `vfilter:"optional,field=end,doc=End scanning at this offset (100mb)"`
	NumberOfHits    int64             `vfilter:"optional,field=number,doc=Stop after this many hits (1)."`
	Blocksize       uint64            `vfilter:"optional,field=blocksize,doc=Blocksize for scanning (10mb)."`
	Key             string            `vfilter:"optional,field=key,doc=If set use this key to cache the  yara rules."`
	Namespace       string            `vfilter:"optional,field=namespace,doc=The Yara namespece to use."`
	YaraVariables   *ordereddict.Dict `vfilter:"optional,field=vars,doc=The Yara variables to use."`
	YaraXDLLPath    *vfilter.Lambda   `vfilter:"required,field=dll_path,doc=Function to resolve path to the yarax DLL"`
	ForceBufferScan bool              `vfilter:"optional,field=force_buffers,doc=Force buffer scan in all cases."`
}

type YaraXScanPlugin struct{}

func (self YaraXScanPlugin) Call(
	ctx context.Context,
	scope vfilter.Scope,
	args *ordereddict.Dict) <-chan vfilter.Row {
	output_chan := make(chan vfilter.Row)

	go func() {
		defer close(output_chan)
		defer vql_subsystem.RegisterMonitor(ctx, "yarax", args)()

		arg := &YaraXScanPluginArgs{}
		err := arg_parser.ExtractArgsWithContext(ctx, scope, args, arg)
		if err != nil {
			scope.Log("yarax: %v", err)
			return
		}

		if !yara_x.IsLoaded() {
			yara_dll_path := utils.ToString(arg.YaraXDLLPath.Reduce(
				ctx, scope, []vfilter.Any{scope}))

			// Make sure the path is absolute as we dont want any dll
			// hijacking possibility.
			if !filepath.IsAbs(yara_dll_path) {
				scope.Error("yarax: dll path must be an absolute path, not %v",
					yara_dll_path)
				return
			}

			// Attempt to load the dll if needed.
			res := yara_x.LoadYaraXDLL(yara_dll_path)
			if res != "" {
				scope.Error("yarax: %v", res)
				return
			}
		}

		if arg.NumberOfHits == 0 {
			arg.NumberOfHits = 1
		}

		if arg.Blocksize == 0 {
			arg.Blocksize = 10 * 1024 * 1024
		}

		rules, err := yaraXgetYaraRules(arg.Key, arg.Namespace, arg.Rules,
			arg.YaraVariables, scope)
		if err != nil {
			functions.DeduplicatedLog(ctx, scope, "ERROR:yarax: "+err.Error())
			return
		}

		matcher := &yaraXscanReporter{
			output_chan:    output_chan,
			blocksize:      arg.Blocksize,
			number_of_hits: arg.NumberOfHits,
			context:        arg.Context,
			ctx:            ctx,
			rules:          rules,
			scope:          scope,
		}

		accessor, err := accessors.GetAccessor(arg.Accessor, scope)
		if err != nil {
			scope.Error("yara: %v", err)
			return
		}

		for _, filename_any := range arg.Files {
			filename, err := accessors.ParseOSPath(ctx, scope, accessor, filename_any)
			if err != nil {
				scope.Log("yara: %v", err)
				continue
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
						scope.Log("yarax: Directly scanning file %v failed, will use accessor",
							filename.String())
					}
				}
			}

			matcher.scanFileByAccessor(ctx, arg.Accessor, accessor,
				arg.Blocksize, arg.Start, arg.End, output_chan)
		}
	}()

	return output_chan
}

func yaraXgetYaraRules(key, namespace, rules string,
	vars *ordereddict.Dict, scope vfilter.Scope) (*yara_x.Rules, error) {

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

		case *yara_x.Rules:
			return t, nil

		default:
			// Unknown type - recompile again.
		}
	}

	compiled_rules, err := yaraXcompileRules(
		scope, vars, key, namespace, rules)
	if err != nil {
		vql_subsystem.CacheSet(scope, key, err)
		return nil, err
	}

	// Cache the successful rules for further use
	vql_subsystem.CacheSet(scope, key, compiled_rules)
	return compiled_rules, nil
}

func yaraXcompileRules(scope vfilter.Scope,
	vars *ordereddict.Dict,
	key, namespace, rules string) (*yara_x.Rules, error) {

	var opts []yara_x.CompileOption
	compiler, err := yara_x.NewCompiler(opts...)
	if err != nil {
		return nil, err
	}

	compiler.NewNamespace(namespace)

	if vars != nil {
		for _, i := range vars.Items() {
			err := compiler.DefineGlobal(i.Key, i.Value)
			if err != nil {
				vql_subsystem.CacheSet(scope, i.Key, err)
				return nil, err
			}
		}
	}

	err = compiler.AddSource(rules)
	if err != nil {
		// Cache the compile failure so only one log is emitted.
		vql_subsystem.CacheSet(scope, key, err)
		return nil, err
	}

	return compiler.Build(), nil

}

// Scan a file called when no accessor is specified. We pass the
// filename to libyara directly for faster scanning using mmap. This
// also ensures that all yara features (like the PE plugin) work.
func (self *yaraXscanReporter) scanFile(
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

	scanner := yara_x.NewScanner(self.rules)

	// Do not wait for the GC to cleanup.
	defer scanner.Destroy()

	scanner.SetTimeout(100 * time.Second)

	// There is no way to actively cancel the yara scan so this is as
	// good as we can get - do not set the timeout too long or we wont
	// be able to cancel it promptly.
	result, err := scanner.ScanFile(underlying_file)
	if err != nil {
		return err
	}

	for _, hit := range result.MatchingRules() {
		self.ReportMatch(hit)
	}

	// We count an op as one file scanned.
	self.scope.ChargeOp()

	return nil
}

func (self *yaraXscanReporter) scanFileByAccessor(
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

	scanner := yara_x.NewScanner(self.rules)

	// Do not wait for the GC to cleanup.
	defer scanner.Destroy()

	scanner.SetTimeout(100 * time.Second)

	self.file_info, err = accessor.LstatWithOSPath(self.filename)
	if err != nil {
		self.scope.Log("yara: Failed to open %v with accessor %v: %v",
			self.filename, accessor_name, err)
		return
	}
	self.reader = utils.MakeReaderAtter(f)

	// If the file is larger than the block size, we need to scan in
	// block mode.

	// NOTE: When scanning in block mode, many yara modules are
	// disabled, such as "hash" or "pe".

	// https://github.com/VirusTotal/yara-x/blob/c2a09cc60017856c49de84b41bbaba378850c704/capi/include/yara_x.h#L726

	// 1) Modules won't work. Parsers for structured formats (e.g.,
	//    PE, ELF) require access to the entire file and cannot be
	//    applied in block scanning mode.

	// 2) Other modules like `hash` won't work either, as they require
	//    access to all the scanned data during the evaluation of the
	//    rule's condition, something that can't be guaranteed in
	//    block scanning mode. The hash functions will return
	//    `undefined` when used in a multi-block context.

	// 3) Built-in functions like `uint8`, `uint16`, `uint32`, etc.,
	//    have the same limitation. They also return `undefined` in
	//    block scanning mode.

	// 4) The `filesize` keyword returns `undefined` in block scanning
	//    mode.

	// 5) Patterns won't match across block boundaries. Every match
	//    will be completely contained within one of the blocks.

	block_scanning_mode := self.file_info.Size() > int64(self.blocksize) ||
		// If we need to start mid way through the file, we have to
		// scan in blocks.
		start > 0

	// In block scanning mode we get all the hits when we finished
	// scanning the file instead.
	if block_scanning_mode {
		defer func() {
			result, err := scanner.Finish()
			if err != nil {
				self.scope.Log("yarax: %v", err)
				return
			}

			for _, hit := range result.MatchingRules() {
				self.ReportMatch(hit)
			}
		}()
	}

	// Support sparse file scanning
	range_reader, ok := f.(uploads.RangeReader)
	if !ok {
		// File does not support ranges, just cap the end at
		// the end of the file.
		if end == 0 && self.file_info != nil {
			end = uint64(self.file_info.Size())
		}

		self.scanRange(start, end, scanner, f, block_scanning_mode)
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

			self.scanRange(scan_start, scan_end, scanner,
				f, block_scanning_mode)
		}
	}
}

func (self *yaraXscanReporter) scanRange(
	start, end uint64,
	scanner *yara_x.Scanner,
	f accessors.ReadSeekCloser,
	scan_in_block_mode bool) {
	buf := make([]byte, self.blocksize)

	// self.scope.Log("Scanning %v from %#0x to %#0x", self.filename, start, end)

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

		n, err := f.Read(buf[:to_read])
		if n == 0 {
			return
		}

		scan_buf := buf[:n]
		var result *yara_x.ScanResults

		if scan_in_block_mode {
			// Do an additional scan on the first block. Yara X
			// disable a lot of functionality in block scanning mode,
			// while many rules depend on it. This attempts a non
			// block scan on the first block.
			if self.base_offset == 0 {
				result, err = scanner.Scan(scan_buf)
				if err != nil {
					self.scope.Log("yarax: %v", err)
					return
				}

				hits := result.MatchingRules()
				if len(hits) > 0 {
					for _, hit := range hits {
						self.ReportMatch(hit)
					}
					continue
				}
			}

			result, err = scanner.ScanBlock(scan_buf, self.base_offset)
			if err != nil {
				self.scope.Log("yarax: %v", err)
				return
			}

		} else {
			result, err = scanner.Scan(scan_buf)
			if err != nil {
				self.scope.Log("yarax: %v", err)
				return
			}
		}

		for _, hit := range result.MatchingRules() {
			self.ReportMatch(hit)
		}

		// Advance the read pointer
		self.base_offset += uint64(n)

		// We count an op as one MB scanned.
		self.scope.ChargeOp()
	}
}

// Reports all hits in the match and includes any required context. We
// report one row per matching string in the signature, unless no
// strings match in which case we report a single row for the
// match. The number_of_hits parameter specified how many hits the
// caller is interested in so we can breakout early. This returns the
// total number of hits actually reported.
type yaraXscanReporter struct {
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

	// Internal scan state
	scope            vfilter.Scope
	rules            *yara_x.Rules
	fast_buffer_scan bool
}

func (self *yaraXscanReporter) getMeta(rule *yara_x.Rule) *ordereddict.Dict {
	metas := rule.Metadata()
	if len(metas) > 0 {
		result := ordereddict.NewDict()
		for _, m := range metas {
			result.Set(m.Identifier(), m.Value())
		}
		return result
	}
	return nil
}

func (self *yaraXscanReporter) ReportMatch(rule *yara_x.Rule) {
	matches := yaraXgetMatchStrings(rule)
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
			return

		case self.output_chan <- res:
		}

		self.number_of_hits--
		return
	}

	// If the rule matches multiple strings, emit one string per row.
	for _, match_string := range matches {

		// Extract a larger context from the scan buffer.
		context_start := int64(match_string.Offset) - int64(self.context)

		if context_start < 0 {
			context_start = 0
		}

		context_end := int64(match_string.Offset) +
			int64(match_string.Length) + int64(self.context)

		// Make a copy of the underlying data.
		data := make([]byte, context_end-context_start)
		n, _ := self.reader.ReadAt(data, int64(context_start))
		data = data[:n]

		res := &YaraResult{
			Rule:     rule.Identifier(),
			Tags:     rule.Tags(),
			Meta:     metas,
			File:     self.file_info,
			FileName: self.filename,
			String: &YaraHit{
				Name:    match_string.Name,
				Offset:  match_string.Offset,
				Data:    data,
				HexData: strings.Split(hex.Dump(data), "\n"),
			},
		}

		if self.end > 0 && res.String.Offset >= self.end {
			return
		}

		// Emit the results.
		select {
		case <-self.ctx.Done():
			return

		case self.output_chan <- res:
		}

		self.number_of_hits--
		if self.number_of_hits <= 0 {
			return
		}

	}
}

type MatchString struct {
	Name   string
	Offset uint64
	Length uint64
}

func yaraXgetMatchStrings(r *yara_x.Rule) (matchstrings []MatchString) {
	for _, s := range r.Patterns() {
		for _, m := range s.Matches() {
			matchstrings = append(matchstrings, MatchString{
				Name:   s.Identifier(),
				Offset: uint64(m.Offset()),
				Length: uint64(m.Length()),
			})
		}
	}
	return
}

func (self YaraXScanPlugin) Info(
	scope vfilter.Scope,
	type_map *vfilter.TypeMap) *vfilter.PluginInfo {
	return &vfilter.PluginInfo{
		Name:    "yarax",
		Doc:     "Scan files using yara rules (Using the new yarax engine).",
		ArgType: type_map.AddType(scope, &YaraXScanPluginArgs{}),

		// EXECVE is needed because we load the yarax DLL dynamically.
		Metadata: vql.VQLMetadata().Permissions(
			acls.FILESYSTEM_READ, acls.EXECVE).Build(),
	}
}

func init() {
	vql_subsystem.RegisterPlugin(&YaraXScanPlugin{})
}
