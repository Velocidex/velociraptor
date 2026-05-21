package shell

import (
	"context"

	"github.com/Velocidex/ordereddict"
	"www.velocidex.com/golang/velociraptor/acls"
	"www.velocidex.com/golang/velociraptor/artifacts"
	vql_subsystem "www.velocidex.com/golang/velociraptor/vql"
	vfilter "www.velocidex.com/golang/vfilter"
	"www.velocidex.com/golang/vfilter/arg_parser"
)

type ShellSessionControlArgs struct {
	Name       string `vfilter:"optional,field=name,doc=The name of the shell session."`
	Stdin      string `vfilter:"optional,field=stdin,doc=Write this string to the session stdin."`
	StdinClose bool   `vfilter:"optional,field=close_stdin,doc=If specified we close the stdin of the session. This will usually terminate the session gracefully."`
	Kill       bool   `vfilter:"optional,field=kill,doc=If specified we kill the session."`
}

type ShellSessionControl struct{}

func (self *ShellSessionControl) Call(ctx context.Context,
	scope vfilter.Scope,
	args *ordereddict.Dict) vfilter.Any {
	err := vql_subsystem.CheckAccess(scope, acls.EXECVE)
	if err != nil {
		scope.Log("shell_session_control: %v", err)
		return vfilter.Null{}
	}

	// Check the config if we are allowed to execve at all.
	config_obj, ok := artifacts.GetConfig(scope)
	if ok && config_obj.PreventExecve {
		scope.Log("shell_session_control: Not allowed to execve by configuration.")
		return vfilter.Null{}
	}

	arg := &ShellSessionControlArgs{}
	err = arg_parser.ExtractArgsWithContext(ctx, scope, args, arg)
	if err != nil {
		scope.Error("shell_session_control: %v", err)
		return vfilter.Null{}
	}

	session, pres := shellManager.GetByName(arg.Name)
	if !pres {
		scope.Error("shell_session_control: session %v not known", arg.Name)
		return vfilter.Null{}
	}

	if arg.Stdin != "" {
		session.WriteStdin(ctx, arg.Stdin)
	}

	if arg.StdinClose {
		session.CloseStdin()
	}

	return session
}

func (self ShellSessionControl) Info(scope vfilter.Scope, type_map *vfilter.TypeMap) *vfilter.FunctionInfo {
	return &vfilter.FunctionInfo{
		Name:     "shell_session_control",
		Doc:      "Control a previously created shell session.",
		ArgType:  type_map.AddType(scope, &ShellSessionControlArgs{}),
		Metadata: vql_subsystem.VQLMetadata().Permissions(acls.EXECVE).Build(),
		Version:  1,
	}
}

func init() {
	vql_subsystem.RegisterFunction(&ShellSessionControl{})
}
