// +build cgo

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
	"context"
	"crypto/md5"
	"encoding/hex"
	"os"
	"strings"
	"time"

	"github.com/Velocidex/go-yara"
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
	Rule    string
	Meta    map[string]interface{}
	Tags    []string
	Strings []*YaraHit
	File    os.FileInfo
}

type YaraScanPluginArgs struct {
	Rules        string   `vfilter:"required,field=rules,doc=Yara rules in the yara DSL."`
	Files        []string `vfilter:"required,field=files,doc=The list of files to scan."`
	Accessor     string   `vfilter:"optional,field=accessor,doc=Accessor (e.g. NTFS)"`
	Context      int      `vfilter:"optional,field=context,doc=How many bytes to include around each hit"`
	Start        int64    `vfilter:"optional,field=start,doc=The start offset to scan"`
	End          uint64   `vfilter:"optional,field=end,doc=End scanning at this offset (100mb)"`
	NumberOfHits int64    `vfilter:"optional,field=number,doc=Stop after this many hits (1)."`
	Blocksize    int64    `vfilter:"optional,field=blocksize,doc=Blocksize for scanning (1mb)."`
}

type YaraScanPlugin struct{}

func (self YaraScanPlugin) Call(
	ctx context.Context,
	scope *vfilter.Scope,
	args *vfilter.Dict) <-chan vfilter.Row {
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

		// Try to get the compiled yara expression from the
		// scope cache.
		rule_hash := md5.Sum([]byte(arg.Rules))
		rule_hash_str := string(rule_hash[:])
		rules, ok := vql_subsystem.CacheGet(
			scope, "yara_rule"+rule_hash_str).(*yara.Rules)
		if !ok {
			variables := make(map[string]interface{})
			rules, err = yara.Compile(arg.Rules, variables)
			if err != nil {
				scope.Log("Failed to initialize YARA compiler: %s", err)
				return
			}
			vql_subsystem.CacheSet(
				scope, "yara_rule"+rule_hash_str, rules)
		}

		accessor := glob.GetAccessor(arg.Accessor, ctx)
		buf := make([]byte, arg.Blocksize)

	scan_file:
		for _, filename := range arg.Files {
			// Total hits per file.
			number_of_hits := int64(0)
			f, err := accessor.Open(filename)
			if err != nil {
				scope.Log("Failed to open %v", filename)
				continue
			}
			f.Seek(arg.Start, 0)
			base_offset := uint64(arg.Start)
			for {
				n, _ := f.Read(buf)
				if n == 0 {
					break
				}

				matches, err := rules.ScanMem(
					buf[:n], yara.ScanFlagsFastMode,
					10*time.Second)
				if err != nil {
					break
				}

				for _, match := range matches {
					rule := match.Rule
					if match.Namespace != "default" {
						rule = match.Namespace + ":" + rule
					}

					stat, _ := f.Stat()
					res := &YaraResult{
						Rule: rule,
						Tags: match.Tags,
						Meta: match.Meta,
						File: stat,
					}

					for _, match_string := range match.Strings {
						start := int(match_string.Offset) -
							arg.Context
						if start < 0 {
							start = 0
						}

						end := int(match_string.Offset) +
							len(match_string.Data) +
							arg.Context
						if end >= len(buf) {
							end = len(buf) - 1
						}

						// Make a copy of the underlying data.
						data := make([]byte, end-start)
						copy(data, buf[start:end])

						res.Strings = append(
							res.Strings, &YaraHit{
								Name: match_string.Name,
								Offset: match_string.Offset +
									base_offset,
								Data: data,
								HexData: strings.Split(
									hex.Dump(data), "\n"),
							})
					}
					number_of_hits += 1
					if number_of_hits > arg.NumberOfHits {
						f.Close()
						continue scan_file
					}
					output_chan <- res
				}

				base_offset += uint64(n)
				if base_offset > arg.End {
					break
				}

				// We count an op as one MB scanned.
				vfilter.ChargeOp(scope)
			}

			f.Close()
		}
	}()

	return output_chan
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
	Rules   string `vfilter:"required,field=rules"`
	Pid     int    `vfilter:"required,field=pid"`
	Context int    `vfilter:"optional,field=context"`
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
	args *vfilter.Dict) <-chan vfilter.Row {
	output_chan := make(chan vfilter.Row)

	go func() {
		defer close(output_chan)

		arg := &YaraProcPluginArgs{}
		err := vfilter.ExtractArgs(scope, args, arg)
		if err != nil {
			scope.Log("proc_yara: %v", err)
			return
		}

		variables := make(map[string]interface{})
		rules, err := yara.Compile(arg.Rules, variables)
		if err != nil {
			scope.Log("Failed to initialize YARA compiler: %v", err)
			return
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

func init() {
	vql_subsystem.RegisterPlugin(&YaraScanPlugin{})
	vql_subsystem.RegisterPlugin(&YaraProcPlugin{})
}
