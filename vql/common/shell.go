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
	"fmt"
	"io"
	"os/exec"
	"strings"
	"sync"
	"syscall"

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
	Stdout     string
	Stderr     string
	ReturnCode int64
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
		stdout_pipe, err := command.StdoutPipe()
		if err != nil {
			scope.Log("shell: no command to run")
			return
		}

		stderr_pipe, err := command.StderrPipe()
		if err != nil {
			scope.Log("shell: no command to run")
			return
		}

		err = command.Start()
		if err != nil {
			scope.Log("shell: %v", err)
			output_chan <- &ShellResult{
				ReturnCode: 1,
				Stderr:     fmt.Sprintf("%v", err),
			}
			return

		}

		// We need to combine the status code with the stdout
		// to minimize the total number of responses.  Send a
		// copy of the response because we will continue
		// modifying it.
		response := ShellResult{}
		wg := sync.WaitGroup{}

		read_from_pipe := func(
			pipe io.ReadCloser,
			output_member *string,
			wg *sync.WaitGroup) {

			wg.Add(1)
			defer wg.Done()

			// Read as much as possible into the buffer
			// filling the full length - even if we have
			// to wait on the pipe.
			buff := make([]byte, arg.Length)
			offset := 0

			for {
				select {
				case <-ctx.Done():
					return

				default:
					n, err := pipe.Read(buff[offset:])
					if err != nil && err != io.EOF {
						scope.Log("shell: %v", err)
						return
					}

					// Read some data into the buffer.
					if n > 0 {
						offset += n
						continue
					}

					// The buffer is completely empty and
					// the last read was an EOF.
					if n == 0 && offset == 0 && err == io.EOF {
						return
					}
					if arg.Sep != "" {
						for _, line := range strings.Split(
							string(buff[:offset]), arg.Sep) {
							if len(*output_member) > 0 {
								output_chan <- response
							}

							*output_member = line
						}

					} else {
						if len(*output_member) > 0 {
							output_chan <- response
						}

						*output_member = string(buff[:offset])
					}

					// Write over the same buffer with new data.
					offset = 0
				}
			}
		}

		// Read asyncronously.
		go read_from_pipe(stdout_pipe, &response.Stdout, &wg)
		go read_from_pipe(stderr_pipe, &response.Stderr, &wg)

		// We need to wait here until the readers are done.
		wg.Wait()

		// Get the command status and combine with the last response.
		err = command.Wait()
		if err == nil {
			// Successful termination.
			response.ReturnCode = 0
		} else {
			response.ReturnCode = -1

			exiterr, ok := err.(*exec.ExitError)
			if ok {
				status, ok := exiterr.Sys().(syscall.WaitStatus)
				if ok {
					response.ReturnCode = int64(status.ExitStatus())
				}
			}
		}

		output_chan <- response
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
