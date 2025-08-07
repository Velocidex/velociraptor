package functions

import (
	"context"

	"github.com/Velocidex/ordereddict"
	"github.com/google/shlex"
	vql_subsystem "www.velocidex.com/golang/velociraptor/vql"
	"www.velocidex.com/golang/vfilter"
	"www.velocidex.com/golang/vfilter/arg_parser"
)

type CommandlineToArgvArgs struct {
	Command   string `vfilter:"required,field=command,doc=Commandline to split into components."`
	BashStyle bool   `vfilter:"optional,field=bash_style,doc=Use bash rules (Uses Windows rules by default)."`
}

type CommandlineToArgvFunction struct{}

func (self *CommandlineToArgvFunction) Call(ctx context.Context,
	scope vfilter.Scope,
	args *ordereddict.Dict) vfilter.Any {

	defer vql_subsystem.RegisterMonitor(ctx, "commandline", args)()

	arg := &CommandlineToArgvArgs{}
	err := arg_parser.ExtractArgsWithContext(ctx, scope, args, arg)
	if err != nil {
		scope.Log("commandline_split: %v", err)
		return []string{}
	}

	if arg.BashStyle {
		res, err := shlex.Split(arg.Command)
		if err != nil {
			scope.Log("commandline_split: %v", err)
			return []string{}
		}
		return res
	}

	return CommandLineToArgv(arg.Command)
}

func (self CommandlineToArgvFunction) Info(scope vfilter.Scope, type_map *vfilter.TypeMap) *vfilter.FunctionInfo {
	return &vfilter.FunctionInfo{
		Name:    "commandline_split",
		Doc:     "Split a commandline into separate components following the windows convensions.",
		ArgType: type_map.AddType(scope, &CommandlineToArgvArgs{}),
	}
}

func init() {
	vql_subsystem.RegisterFunction(&CommandlineToArgvFunction{})
}

// From https://github.com/golang/go/blob/master/src/os/exec_windows.go

// appendBSBytes appends n '\\' bytes to b and returns the resulting slice.
func appendBSBytes(b []byte, n int) []byte {
	for ; n > 0; n-- {
		b = append(b, '\\')
	}
	return b
}

// readNextArg splits command line string cmd into next
// argument and command line remainder.
func readNextArg(cmd string) (arg []byte, rest string) {
	var b []byte
	var inquote bool
	var nslash int
	for ; len(cmd) > 0; cmd = cmd[1:] {
		c := cmd[0]
		switch c {
		case ' ', '\t':
			if !inquote {
				return appendBSBytes(b, nslash), cmd[1:]
			}
		case '"':
			b = appendBSBytes(b, nslash/2)
			if nslash%2 == 0 {
				// use "Prior to 2008" rule from
				// http://daviddeley.com/autohotkey/parameters/parameters.htm
				// section 5.2 to deal with double double quotes
				if inquote && len(cmd) > 1 && cmd[1] == '"' {
					b = append(b, c)
					cmd = cmd[1:]
				}
				inquote = !inquote
			} else {
				b = append(b, c)
			}
			nslash = 0
			continue
		case '\\':
			nslash++
			continue
		}
		b = appendBSBytes(b, nslash)
		nslash = 0
		b = append(b, c)
	}
	return appendBSBytes(b, nslash), ""
}

// commandLineToArgv splits a command line into individual argument
// strings, following the Windows conventions documented
// at http://daviddeley.com/autohotkey/parameters/parameters.htm#WINARGV
func CommandLineToArgv(cmd string) []string {
	var args []string
	for len(cmd) > 0 {
		if cmd[0] == ' ' || cmd[0] == '\t' {
			cmd = cmd[1:]
			continue
		}
		var arg []byte
		arg, cmd = readNextArg(cmd)
		args = append(args, string(arg))
	}
	return args
}
