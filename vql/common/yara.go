package common

import (
	"context"
	"encoding/hex"
	"strings"
	"time"

	"github.com/Velocidex/go-yara"
	"www.velocidex.com/golang/velociraptor/glob"
	vql_subsystem "www.velocidex.com/golang/velociraptor/vql"
	vfilter "www.velocidex.com/golang/vfilter"
)

var (
	BUFFSIZE = 1024 * 1024
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
}

type YaraScanPluginArgs struct {
	Rules    string   `vfilter:"required,field=rules"`
	Files    []string `vfilter:"required,field=files"`
	Accessor string   `vfilter:"optional,field=accessor"`
	Context  int      `vfilter:"optional,field=context"`
	Start    int64    `vfilter:"optional,field=start"`
	End      uint64   `vfilter:"optional,field=end"`
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

		variables := make(map[string]interface{})
		rules, err := yara.Compile(arg.Rules, variables)
		if err != nil {
			scope.Log("Failed to initialize YARA compiler: %s", err)
			return
		}

		accessor := glob.GetAccessor(arg.Accessor, ctx)
		buf := make([]byte, BUFFSIZE)
		for _, filename := range arg.Files {
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

					res := &YaraResult{
						Rule: rule,
						Tags: match.Tags,
						Meta: match.Meta,
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

						data := buf[start:end]

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

					output_chan <- res
				}

				base_offset += uint64(n)
				if base_offset > arg.End {
					break
				}

			}

			f.Close()
		}
	}()

	return output_chan
}

func (self YaraScanPlugin) Info(type_map *vfilter.TypeMap) *vfilter.PluginInfo {
	return &vfilter.PluginInfo{
		Name:    "yara",
		Doc:     "Scan files using yara rules.",
		ArgType: "YaraScanPluginArgs",
	}
}

type YaraProcPluginArgs struct {
	Rules   string `vfilter:"required,field=rules"`
	Pid     int    `vfilter:"required,field=pid"`
	Context int    `vfilter:"optional,field=context"`
}

type YaraProcPlugin struct{}

func (self YaraProcPlugin) Info(type_map *vfilter.TypeMap) *vfilter.PluginInfo {
	return &vfilter.PluginInfo{
		Name:    "proc_yara",
		Doc:     "Scan processes using yara rules.",
		ArgType: "YaraProcPluginArgs",
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

	}()

	return output_chan
}

func init() {
	vql_subsystem.RegisterPlugin(&YaraScanPlugin{})
	vql_subsystem.RegisterPlugin(&YaraProcPlugin{})
}
