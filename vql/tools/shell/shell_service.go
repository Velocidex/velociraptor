package shell

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os/exec"
	"sync"
	"time"

	"github.com/Velocidex/ordereddict"
	"www.velocidex.com/golang/velociraptor/acls"
	"www.velocidex.com/golang/velociraptor/artifacts"
	"www.velocidex.com/golang/velociraptor/utils"
	vql_subsystem "www.velocidex.com/golang/velociraptor/vql"
	vfilter "www.velocidex.com/golang/vfilter"
	"www.velocidex.com/golang/vfilter/arg_parser"
	"www.velocidex.com/golang/vfilter/types"
)

var (
	shellManager = &ShellManager{
		sessions: map[string]*ShellSession{},
	}

	closeChannelMagic = fmt.Sprintf("%v", utils.GetGUID())
)

type ShellManager struct {
	mu       sync.Mutex
	sessions map[string]*ShellSession
}

func (self *ShellManager) Remove(name string) {
	self.mu.Lock()
	defer self.mu.Unlock()

	delete(self.sessions, name)
}

func (self *ShellManager) GetByName(name string) (*ShellSession, bool) {
	self.mu.Lock()
	defer self.mu.Unlock()

	res, pres := self.sessions[name]
	return res, pres
}

func (self *ShellManager) Get(
	ctx context.Context, scope vfilter.Scope,
	args *StartShellFunctionArgs) (*ShellSession, error) {
	self.mu.Lock()
	defer self.mu.Unlock()

	res, pres := self.sessions[args.Name]
	if pres {
		return res, nil
	}

	res, err := self.newShellSession(ctx, scope, args)
	if err != nil {
		return nil, err
	}

	self.sessions[args.Name] = res
	return res, err
}

func (self *ShellManager) mergeSecretToRequest(
	ctx context.Context, scope vfilter.Scope,
	arg *StartShellFunctionArgs, secret_name string) (*StartShellFunctionArgs, error) {
	execve_args := &ShellPluginArgs{
		Argv:   arg.Argv,
		Env:    arg.Env,
		Cwd:    arg.Cwd,
		Secret: arg.Secret}

	res, err := mergeSecretToRequest(ctx, scope, execve_args, arg.Secret)
	if err != nil {
		return nil, err
	}

	return &StartShellFunctionArgs{
		Argv: res.Argv,
		Env:  res.Env,
		Cwd:  res.Cwd,
	}, nil
}

func (self *ShellManager) newShellSession(
	ctx context.Context, scope vfilter.Scope,
	arg *StartShellFunctionArgs) (res *ShellSession, err error) {

	if len(arg.Argv) == 0 {
		return nil, errors.New("Argv not provided")
	}

	original_args := append([]string{}, arg.Argv...)
	if arg.Secret != "" {
		arg, err = self.mergeSecretToRequest(
			ctx, scope, arg, arg.Secret)
		if err != nil {
			return nil, err
		}
	}

	output_chan := make(chan types.Row)
	wg := &sync.WaitGroup{}

	sub_ctx, cancel := context.WithCancel(ctx)
	err = scope.AddDestructor(cancel)
	if err != nil {
		cancel()
		return
	}

	scope.Log("shell_session: Running external command %v", original_args)
	command := exec.CommandContext(sub_ctx, arg.Argv[0], arg.Argv[1:]...)
	if arg.Env != nil {
		for _, i := range arg.Env.Items() {
			command.Env = append(command.Env,
				fmt.Sprintf("%s=%s", i.Key, i.Value))
		}
	}
	command.Dir = arg.Cwd
	command.WaitDelay = time.Second
	command.Cancel = func() error {
		cancel()
		return nil
	}
	UpdateCommandForOS(command)

	stdin_pipe, err := command.StdinPipe()
	if err != nil {
		return nil, err
	}

	stdout_pipe, err := command.StdoutPipe()
	if err != nil {
		return nil, err
	}

	stderr_pipe, err := command.StderrPipe()
	if err != nil {
		return nil, err
	}

	input_chan := make(chan string, 10)

	res = &ShellSession{
		Name:    arg.Name,
		wg:      wg,
		owner:   self,
		cancel:  cancel,
		input:   input_chan,
		output:  output_chan,
		command: command,
		stdin:   stdin_pipe,
		stdout:  stdout_pipe,
		stderr:  stderr_pipe,
	}

	wg.Add(1)
	go pumpFromPipeToOutput(sub_ctx, wg, stdout_pipe, func(data []byte) {
		select {
		case <-sub_ctx.Done():
			return
		case output_chan <- &ShellResult{
			Stdout: string(data),
		}:
		}
	})

	wg.Add(1)
	go pumpFromPipeToOutput(sub_ctx, wg, stderr_pipe, func(data []byte) {
		select {
		case <-sub_ctx.Done():
			return
		case output_chan <- &ShellResult{
			Stderr: string(data),
		}:
		}
	})

	// Also report any stdin messages to the output channel. This
	// ensures they are delivered in order.
	wg.Add(1)
	go pumpToPipeFromInput(sub_ctx, wg, input_chan, func(data string) {
		if data == closeChannelMagic {
			res.Close()
			return
		}

		select {
		case <-sub_ctx.Done():
			return
		case output_chan <- &ShellResult{
			Stdin: data,
		}:
			stdin_pipe.Write([]byte(data))
		}
	})

	err = command.Start()
	if err != nil {
		return nil, err
	}

	// Wait for the command to exit then quit all the contexts and
	// pumping routines.
	wg.Add(1)
	go func() {
		defer wg.Done()

		command.Wait()
	}()

	// When all the pumping functions are finished, then close the
	// output channel to finish the query.
	go func() {
		defer cancel()

		wg.Wait()
		close(res.output)
	}()

	return res, nil
}

// A shell is a persistent session
type ShellSession struct {
	mu sync.Mutex

	wg         *sync.WaitGroup
	cancel     func()
	_IsRunning bool
	Name       string
	owner      *ShellManager

	// Shell output will be pushed to this channel. SELECTing from
	// ShellSession will read from this channel.
	stdin   io.WriteCloser
	stdout  io.ReadCloser
	stderr  io.ReadCloser
	command *exec.Cmd
	closed  bool

	input  chan string
	output chan vfilter.Row
}

func (self *ShellSession) WriteStdin(ctx context.Context, data string) {
	self.mu.Lock()
	closed := self.closed
	self.mu.Unlock()
	if closed {
		return
	}

	select {
	case <-ctx.Done():
		return
	case self.input <- data:
	}
}

func (self *ShellSession) CloseStdin() {
	self.input <- closeChannelMagic
}

type StartShellFunctionArgs struct {
	Argv   []string          `vfilter:"required,field=argv,doc=Argv to run the command with."`
	Env    *ordereddict.Dict `vfilter:"optional,field=env,doc=Environment variables to launch with."`
	Cwd    string            `vfilter:"optional,field=cwd,doc=If specified we change to this working directory first."`
	Secret string            `vfilter:"optional,field=secret,doc=The name of a secret to use."`
	Name   string            `vfilter:"optional,field=name,doc=The name of the shell session. If the session already exists, we just return a handle to it."`
}

type StartShellFunction struct{}

func (self StartShellFunction) Call(ctx context.Context,
	scope vfilter.Scope,
	args *ordereddict.Dict) vfilter.Any {

	err := vql_subsystem.CheckAccess(scope, acls.EXECVE)
	if err != nil {
		scope.Log("shell_session: %v", err)
		return vfilter.Null{}
	}

	// Check the config if we are allowed to execve at all.
	config_obj, ok := artifacts.GetConfig(scope)
	if ok && config_obj.PreventExecve {
		scope.Log("shell_session: Not allowed to execve by configuration.")
		return vfilter.Null{}
	}

	arg := &StartShellFunctionArgs{}
	err = arg_parser.ExtractArgsWithContext(ctx, scope, args, arg)
	if err != nil {
		scope.Error("shell_session: %v", err)
		return vfilter.Null{}
	}

	res, err := shellManager.Get(ctx, scope, arg)
	if err != nil {
		scope.Error("shell_session: %v", err)
		return vfilter.Null{}
	}

	return res
}

func (self StartShellFunction) Info(scope vfilter.Scope, type_map *vfilter.TypeMap) *vfilter.FunctionInfo {
	return &vfilter.FunctionInfo{
		Name:     "shell_session",
		Doc:      "Recreate or retrieve a shell session handle.",
		ArgType:  type_map.AddType(scope, &StartShellFunctionArgs{}),
		Metadata: vql_subsystem.VQLMetadata().Permissions(acls.EXECVE).Build(),
		Version:  1,
	}
}

func init() {
	vql_subsystem.RegisterFunction(&StartShellFunction{})
}
