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
	"errors"
	"fmt"
	"io"
	"os/exec"
	"runtime"
	"strings"
	"sync"
	"syscall"

	"github.com/Velocidex/ordereddict"
	"github.com/google/shlex"
	"www.velocidex.com/golang/velociraptor/acls"
	"www.velocidex.com/golang/velociraptor/artifacts"
	"www.velocidex.com/golang/velociraptor/constants"
	"www.velocidex.com/golang/velociraptor/services"
	"www.velocidex.com/golang/velociraptor/utils"
	"www.velocidex.com/golang/velociraptor/vql"
	vql_subsystem "www.velocidex.com/golang/velociraptor/vql"
	"www.velocidex.com/golang/velociraptor/vql/functions"
	vfilter "www.velocidex.com/golang/vfilter"
	"www.velocidex.com/golang/vfilter/arg_parser"
)

type ShellPluginArgs struct {
	Argv   []string          `vfilter:"required,field=argv,doc=Argv to run the command with."`
	Sep    string            `vfilter:"optional,field=sep,doc=The separator that will be used to split the stdout into rows."`
	Length int64             `vfilter:"optional,field=length,doc=Size of buffer to capture output per row."`
	Env    *ordereddict.Dict `vfilter:"optional,field=env,doc=Environment variables to launch with."`
	Cwd    string            `vfilter:"optional,field=cwd,doc=If specified we change to this working directory first."`
	Secret string            `vfilter:"optional,field=secret,doc=The name of a secret to use."`
}

type ShellResult struct {
	Stdout     string
	Stderr     string
	ReturnCode int64
	Complete   bool
}

type ShellPlugin struct {
	pipeReader pipeReaderFunc
}

func (self *ShellPlugin) maybeForceSecrets(
	ctx context.Context, scope vfilter.Scope,
	arg *ShellPluginArgs) error {

	// Not running on the server, secrets dont work.
	config_obj, ok := vql_subsystem.GetServerConfig(scope)
	if !ok {
		return nil
	}

	if config_obj.Security == nil {
		return nil
	}

	if !config_obj.Security.VqlMustUseSecrets {
		return nil
	}

	// If an explicit secret is defined let it filter the URLs.
	if arg.Secret != "" {
		return nil
	}

	return utils.SecretsEnforced
}

func (self ShellPlugin) Call(
	ctx context.Context,
	scope vfilter.Scope,
	args *ordereddict.Dict) <-chan vfilter.Row {
	output_chan := make(chan vfilter.Row)

	go func() {
		defer close(output_chan)
		defer vql_subsystem.RegisterMonitor(ctx, "execve", args)()

		err := vql_subsystem.CheckAccess(scope, acls.EXECVE)
		if err != nil {
			scope.Log("execve: %v", err)
			return
		}

		// Check the config if we are allowed to execve at all.
		config_obj, ok := artifacts.GetConfig(scope)
		if ok && config_obj.PreventExecve {
			scope.Log("execve: Not allowed to execve by configuration.")
			return
		}

		arg := &ShellPluginArgs{}
		err = arg_parser.ExtractArgsWithContext(ctx, scope, args, arg)
		if err != nil {
			scope.Error("execve: %v", err)
			return
		}

		err = self.maybeForceSecrets(ctx, scope, arg)
		if err != nil {
			scope.Error("execve: %v", err)
			return
		}

		original_args := append([]string{}, arg.Argv...)
		if arg.Secret != "" {
			arg, err = self.mergeSecretToRequest(ctx, scope, arg, arg.Secret)
			if err != nil {
				scope.Log("ERROR:execve: %v", err)
				return
			}
		}

		if len(arg.Argv) == 0 {
			scope.Log("execve: no command to run")
			return
		}

		// This often happens when people accidentally use VQL
		// expressions to calculate the program name which results in
		// NULL. To avoid a security issue where and attacker can add
		// a binary called "null", we just refuse to run binaries with
		// that name.
		if strings.ToLower(arg.Argv[0]) == "null" {
			scope.Error("execve: Attempt to run NULL, rejected")
			return
		}

		// Kill subprocess when the scope is destroyed.
		sub_ctx, cancel := context.WithCancel(ctx)
		err = scope.AddDestructor(cancel)
		if err != nil {
			// The scope is already cancelled - in that case we just
			// abandon the query and return silently.
			cancel()
			return
		}

		// Report the command we ran for auditing
		// purposes. This will be collected in the flow logs.
		scope.Log("execve: Running external command %v", original_args)

		if arg.Length == 0 {
			arg.Length = 10240
		}

		command := exec.CommandContext(sub_ctx, arg.Argv[0], arg.Argv[1:]...)
		if arg.Env != nil {
			for _, i := range arg.Env.Items() {
				command.Env = append(command.Env,
					fmt.Sprintf("%s=%s", i.Key, i.Value))
			}
		}
		command.Dir = arg.Cwd

		stdout_pipe, err := command.StdoutPipe()
		if err != nil {
			scope.Log("execve: no command to run")
			return
		}

		stderr_pipe, err := command.StderrPipe()
		if err != nil {
			scope.Log("execve: no command to run")
			return
		}

		err = command.Start()
		if err != nil {
			scope.Log("execve: %v", err)
			select {
			case <-ctx.Done():
				return

			case output_chan <- &ShellResult{
				ReturnCode: 1,
				Stderr:     fmt.Sprintf("%v", err),
			}:
			}
			return

		}

		// We need to combine the status code with the stdout to
		// minimize the total number of responses.  Send a copy of the
		// response because we will continue modifying it.
		wg := &sync.WaitGroup{}

		// Read asyncronously.
		var mu sync.Mutex
		response := &ShellResult{}
		length := int(arg.Length)

		wg.Add(1)
		go func() {
			err := self.pipeReader(ctx, stdout_pipe, length, arg.Sep,
				func(line string) {
					mu.Lock()
					defer mu.Unlock()

					if arg.Sep != "" {
						select {
						case <-ctx.Done():
							return

						case output_chan <- &ShellResult{Stdout: line}:
						}

					} else {
						data := response.Stdout + line
						for len(data) > length {
							response.Stdout = data[:length]
							select {
							case <-ctx.Done():
								return

							case output_chan <- &ShellResult{
								Stdout: response.Stdout,
							}:
							}
							data = data[length:]
						}
						response.Stdout = data
					}
				}, wg)
			if err != nil {
				scope.Log("execve: %v", err)
			}
		}()

		wg.Add(1)
		go func() {
			err := self.pipeReader(ctx, stderr_pipe, length, arg.Sep,
				func(line string) {
					mu.Lock()
					defer mu.Unlock()

					if arg.Sep != "" {
						select {
						case <-ctx.Done():
							return

						case output_chan <- &ShellResult{Stdout: line}:
						}
					} else {
						data := response.Stderr + line
						for len(data) > length {
							response.Stderr = data[:length]
							select {
							case <-ctx.Done():
								return

							case output_chan <- &ShellResult{
								Stderr: response.Stderr,
							}:
							}
							data = data[length:]
						}
						response.Stderr = data
					}
				}, wg)
			if err != nil {
				scope.Log("execve: %v", err)
			}
		}()

		// We need to wait here until the readers are done before
		// calling command.Wait.
		wg.Wait()

		// The command has ended and pipes are closed - we just need
		// to get the status message to send it a row.
		mu.Lock()
		defer mu.Unlock()

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
		response.Complete = true

		select {
		case <-ctx.Done():
			return

		case output_chan <- response:
		}
	}()

	return output_chan
}

func (self ShellPlugin) mergeSecretToRequest(
	ctx context.Context, scope vfilter.Scope,
	arg *ShellPluginArgs, secret_name string) (*ShellPluginArgs, error) {

	config_obj, ok := vql_subsystem.GetServerConfig(scope)
	if !ok {
		return nil, errors.New("Secrets may only be used on the server")
	}

	secrets_service, err := services.GetSecretsService(config_obj)
	if err != nil {
		return nil, err
	}

	principal := vql_subsystem.GetPrincipal(scope)
	secret_record, err := secrets_service.GetSecret(ctx, principal,
		constants.EXECVE_SECRET, secret_name)
	if err != nil {
		return nil, err
	}

	// Wipe the args to prevent users from selecting anything here.
	new_arg := &ShellPluginArgs{
		Env: ordereddict.NewDict(),
	}

	new_arg.Env, err = secret_record.GetDict("env")
	if err != nil {
		return nil, fmt.Errorf("Secret %v: While parsing env %w",
			arg.Secret, err)
	}

	// Optional secret variable - it is ok to let the user override.
	secret_record.UpdateString("cwd", &new_arg.Cwd)

	commandline := secret_record.GetString("prefix_commandline")
	if commandline == "" {
		return nil, fmt.Errorf(
			"Secret %v does not specify a prefix_commandline", arg.Secret)
	}

	// Parse the command line into argv
	secret_argv, err := CommandlineToArgv(commandline)
	if err != nil {
		return nil, err
	}

	// Two possibilities:
	// 1. The requested argv matches the secret prefix: This is fine!
	// 2. The requested argv gets added to the secret prefix.
	if len(arg.Argv) >= len(secret_argv) &&
		utils.StringSliceEq(secret_argv, arg.Argv[:len(secret_argv)]) {
		// Prefix matches exactly - no issues - let it run!
		new_arg.Argv = arg.Argv
		return new_arg, nil
	}

	// Add the requested argv to the prefix.
	new_arg.Argv = append(secret_argv, arg.Argv...)

	return new_arg, nil
}

func (self ShellPlugin) Info(scope vfilter.Scope, type_map *vfilter.TypeMap) *vfilter.PluginInfo {
	return &vfilter.PluginInfo{
		Name:     "execve",
		Doc:      "Execute the commands given by argv.",
		ArgType:  type_map.AddType(scope, &ShellPluginArgs{}),
		Metadata: vql.VQLMetadata().Permissions(acls.EXECVE).Build(),
	}
}

type pipeReaderFunc func(
	ctx context.Context,
	pipe io.Reader, buff_length int, sep string,
	cb func(message string), wg *sync.WaitGroup) error

// Split the buffer into seperator lines and push them to the
// callback. Leaved the last part (without the sep) in the buffer for
// next time returning the buffer position where the next read should
// go.
func split(sep string, buff []byte, cb func(message string)) int {
	if len(buff) == 0 {
		return 0
	}

	if sep == "" {
		cb(string(buff))
		return len(buff)
	}

	lines := strings.Split(string(buff), sep)
	if len(lines) > 0 {
		last_line := []byte(lines[len(lines)-1])

		for i := 0; i < len(lines)-1; i++ {
			line := lines[i]
			if len(line) > 0 {
				cb(line)
			}
		}

		// Copy the last line back into the same buffer.
		offset := 0
		for i := 0; i < len(last_line); i++ {
			buff[i] = last_line[i]
			offset++
		}
		return offset
	}
	return 0
}

func defaultPipeReader(
	ctx context.Context,
	pipe io.Reader, buff_length int, sep string,
	cb func(message string), wg *sync.WaitGroup) error {
	defer wg.Done()

	// Read as much as possible into the buffer filling the full
	// length - even if we have to wait on the pipe.
	buff := make([]byte, buff_length)

	// The end of the valid data in the buffer.
	offset := 0

	for {
		select {
		case <-ctx.Done():
			// If there is any data left, send it. Sep can not be in
			// buffer here.
			if offset > 0 {
				cb(string(buff[:offset]))
			}
			return nil

		default:
			// Make sure the buffer has some space.
			if offset >= len(buff) {
				cb(string(buff[:offset]))
				offset = 0
			}

			// From here below it is possible for sep to enter the
			// buffer.
			n, err := pipe.Read(buff[offset:])
			if err == io.EOF {
				offset += n

				// Flush the last of the buffer
				if offset > 0 {
					if sep != "" {
						offset = split(sep, buff[:offset], cb)
					}
					cb(string(buff[:offset]))
				}
				return nil
			}

			if err != nil {
				return err
			}

			// Process the buffer
			offset += n

			// Split the buffer if needed. From here on it is
			// guaranteed that sep is not present in the buffer.
			if sep != "" {
				offset = split(sep, buff[:offset], cb)
				continue
			}

		}
	}
}

func init() {
	vql_subsystem.RegisterPlugin(&ShellPlugin{
		pipeReader: defaultPipeReader,
	})
}

func CommandlineToArgv(in string) ([]string, error) {
	if runtime.GOOS == "windows" {
		return functions.CommandLineToArgv(in), nil
	}

	return shlex.Split(in)
}
