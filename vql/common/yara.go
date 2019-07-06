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
	"fmt"
	"os"
	"strings"
	"time"

	yara "github.com/Velocidex/go-yara"
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
	Start        int64    `vfilter:"optional,field=start,doc=The start offset to scan"`
	End          int64    `vfilter:"optional,field=end,doc=End scanning at this offset (100mb)"`
	NumberOfHits int64    `vfilter:"optional,field=number,doc=Stop after this many hits (1)."`
	Blocksize    int64    `vfilter:"optional,field=blocksize,doc=Blocksize for scanning (1mb)."`
	Key          string   `vfilter:"optional,field=key,doc=If set use this key to cache the  yara rules."`
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
		if arg.Key == "" {
			rule_hash := md5.Sum([]byte(arg.Rules))
			arg.Key = string(rule_hash[:])
		}
		rules, ok := vql_subsystem.CacheGet(
			scope, arg.Key).(*yara.Rules)
		if !ok {
			variables := make(map[string]interface{})
			generated_rules := RuleGenerator(arg.Rules)
			rules, err = yara.Compile(generated_rules, variables)
			if err != nil {
				scope.Log("Failed to initialize YARA compiler: %s", err)
				return
			}
			vql_subsystem.CacheSet(scope, arg.Key, rules)
		}

		accessor, err := glob.GetAccessor(arg.Accessor, ctx)
		if err != nil {
			scope.Log("yara: %v", err)
			return
		}

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
			base_offset := arg.Start
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

						res := &YaraResult{
							Rule:     rule,
							Tags:     match.Tags,
							Meta:     match.Meta,
							File:     stat,
							FileName: filename,
							String: &YaraHit{
								Name: match_string.Name,
								Offset: match_string.Offset +
									uint64(base_offset),
								Data: data,
								HexData: strings.Split(
									hex.Dump(data), "\n"),
							},
						}
						output_chan <- res
						number_of_hits += 1
						if number_of_hits > arg.NumberOfHits {
							f.Close()
							continue scan_file
						}
					}
				}

				base_offset += int64(n)
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

		if arg.Key == "" {
			rule_hash := md5.Sum([]byte(arg.Rules))
			arg.Key = string(rule_hash[:])
		}
		rules, ok := vql_subsystem.CacheGet(
			scope, arg.Key).(*yara.Rules)
		if !ok {
			variables := make(map[string]interface{})
			rules, err := yara.Compile(arg.Rules, variables)
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
func RuleGenerator(rule string) string {
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
