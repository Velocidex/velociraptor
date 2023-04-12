package server

import (
	"context"

	"github.com/Velocidex/ordereddict"
	"www.velocidex.com/golang/velociraptor/acls"
	"www.velocidex.com/golang/velociraptor/services"
	"www.velocidex.com/golang/velociraptor/vql"
	vql_subsystem "www.velocidex.com/golang/velociraptor/vql"
	"www.velocidex.com/golang/vfilter"
	"www.velocidex.com/golang/vfilter/arg_parser"
)

type SendEventArgs struct {
	Artifact string            `vfilter:"required,field=artifact,doc=The artifact name to send the event to."`
	Row      *ordereddict.Dict `vfilter:"required,field=row,doc=The row to send to the artifact"`
}

type SendEventFunction struct{}

func (self *SendEventFunction) Call(ctx context.Context,
	scope vfilter.Scope,
	args *ordereddict.Dict) vfilter.Any {
	arg := &SendEventArgs{}
	err := arg_parser.ExtractArgsWithContext(ctx, scope, args, arg)
	if err != nil {
		scope.Log("send_event: %v", err)
		return &vfilter.Null{}
	}

	// Caller has to be server admin to send events.
	err = vql_subsystem.CheckAccess(scope, acls.SERVER_ADMIN)
	if err != nil {
		// If the user does not have SERVER_ADMIN then maybe they have
		// specific permissions to send to this queue.
		err = vql_subsystem.CheckAccessWithArgs(
			scope, acls.PUBLISH, arg.Artifact)
		if err != nil {
			scope.Log("send_event: %v", err)
			return &vfilter.Null{}
		}
	}

	config_obj, ok := vql_subsystem.GetServerConfig(scope)
	if !ok {
		scope.Log("send_event: Command can only run on the server")
		return &vfilter.Null{}
	}

	journal, err := services.GetJournal(config_obj)
	if err != nil {
		return &vfilter.Null{}
	}

	// We only allow to publish server events - client events come
	// from the client only and not from VQL.
	err = journal.PushRowsToArtifact(ctx, config_obj,
		[]*ordereddict.Dict{arg.Row}, arg.Artifact, "server", "")
	if err != nil {
		scope.Log("send_event: %v", err)
		return &vfilter.Null{}
	}

	return arg.Row
}

func (self SendEventFunction) Info(scope vfilter.Scope, type_map *vfilter.TypeMap) *vfilter.FunctionInfo {
	return &vfilter.FunctionInfo{
		Name:     "send_event",
		Doc:      "Sends an event to a server event monitoring queue.",
		ArgType:  type_map.AddType(scope, &SendEventArgs{}),
		Metadata: vql.VQLMetadata().Permissions(acls.SERVER_ADMIN, acls.PUBLISH).Build(),
	}
}

func init() {
	vql_subsystem.RegisterFunction(&SendEventFunction{})
}
