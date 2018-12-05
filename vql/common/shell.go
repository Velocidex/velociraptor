package common

import (
	"context"
	"io"
	"os/exec"
	"strings"

	api_proto "www.velocidex.com/golang/velociraptor/api/proto"
	vql_subsystem "www.velocidex.com/golang/velociraptor/vql"
	vfilter "www.velocidex.com/golang/vfilter"
)

type ShellPluginArgs struct {
	Argv   []string `vfilter:"required,field=argv"`
	Sep    string   `vfilter:"optional,field=sep"`
	Length int64    `vfilter:"optional,field=length"`
}

type ShellResult struct {
	Stdout string
	Stderr string
}

type ShellPlugin struct{}

func (self ShellPlugin) Call(
	ctx context.Context,
	scope *vfilter.Scope,
	args *vfilter.Dict) <-chan vfilter.Row {
	output_chan := make(chan vfilter.Row)

	go func() {
		defer close(output_chan)

		// Check the config if we are allowed to execve at all.
		scope_config, pres := scope.Resolve("config")
		if pres {
			config_obj, ok := scope_config.(*api_proto.ClientConfig)
			if ok && config_obj.PreventExecve {
				scope.Log("shell: Not allowed to execve by configuration.")
				return
			}
		}

		arg := &ShellPluginArgs{}
		err := vfilter.ExtractArgs(scope, args, arg)
		if err != nil {
			scope.Log("shell: %v", err)
			return
		}

		if len(arg.Argv) == 0 {
			scope.Log("shell: no command to run")
			return
		}

		// Report the command we ran for auditing
		// purposes. This will be collected in the flow logs.
		scope.Log("shell: Running external command %v", arg.Argv)

		if arg.Length == 0 {
			arg.Length = 10240
		}

		command := exec.CommandContext(ctx, arg.Argv[0], arg.Argv[1:]...)
		pipe, err := command.StdoutPipe()
		if err != nil {
			scope.Log("shell: no command to run")
			return
		}

		err = command.Start()
		if err != nil {
			scope.Log("shell: %v", err)
			return

		}

		// Read as much as possible into the buffer filling
		// the full length - even if we have to wait on the
		// pipe.
		buff := make([]byte, arg.Length)
		offset := 0
		for {
			select {
			case <-ctx.Done():
				return
			default:
				n, err := pipe.Read(buff[offset:])
				if err != nil && err != io.EOF {
					return
				}
				if n > 0 {
					offset += n
					continue
				}

				if n == 0 && offset == 0 {
					return
				}

				if arg.Sep != "" {
					for _, line := range strings.Split(
						string(buff[:offset]), arg.Sep) {
						output_chan <- &ShellResult{
							Stdout: line,
						}
					}

				} else {
					output_chan <- &ShellResult{
						Stdout: string(buff[:offset]),
					}
				}

				// Write over the same buffer with new data.
				offset = 0
			}
		}

	}()

	return output_chan
}

func (self ShellPlugin) Info(scope *vfilter.Scope, type_map *vfilter.TypeMap) *vfilter.PluginInfo {
	return &vfilter.PluginInfo{
		Name:    "execve",
		Doc:     "Execute the commands given by argv.",
		ArgType: "ShellPluginArgs",
	}
}

func init() {
	vql_subsystem.RegisterPlugin(&ShellPlugin{})
}
