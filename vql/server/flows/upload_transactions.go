package flows

import (
	"context"

	"github.com/Velocidex/ordereddict"
	"www.velocidex.com/golang/velociraptor/acls"
	"www.velocidex.com/golang/velociraptor/json"
	"www.velocidex.com/golang/velociraptor/services"
	"www.velocidex.com/golang/velociraptor/vql"
	vql_subsystem "www.velocidex.com/golang/velociraptor/vql"
	"www.velocidex.com/golang/vfilter"
	"www.velocidex.com/golang/vfilter/arg_parser"
)

type UploadTransactionsPluginArgs struct {
	ClientId string `vfilter:"required,field=client_id"`
	FlowId   string `vfilter:"optional,field=flow_id"`
}

type UploadTransactionsPlugin struct{}

func (self UploadTransactionsPlugin) Call(
	ctx context.Context,
	scope vfilter.Scope,
	args *ordereddict.Dict) <-chan vfilter.Row {
	output_chan := make(chan vfilter.Row)

	go func() {
		defer close(output_chan)
		defer vql_subsystem.RegisterMonitor(ctx, "upload_transactions", args)()

		err := vql_subsystem.CheckAccess(scope, acls.READ_RESULTS)
		if err != nil {
			scope.Log("upload_transactions: %s", err)
			return
		}

		arg := &UploadTransactionsPluginArgs{}
		err = arg_parser.ExtractArgsWithContext(ctx, scope, args, arg)
		if err != nil {
			scope.Log("upload_transactions: %v", err)
			return
		}

		err = services.RequireFrontend()
		if err != nil {
			scope.Log("upload_transactions: %v", err)
			return
		}

		config_obj, ok := vql_subsystem.GetServerConfig(scope)
		if !ok {
			scope.Log("upload_transactions: Command can only run on the server")
			return
		}

		launcher, err := services.GetLauncher(config_obj)
		if err != nil {
			scope.Log("upload_transactions: %v", err)
			return
		}

		outstanding, err := launcher.ResumeFlow(ctx, config_obj,
			arg.ClientId, arg.FlowId)
		if err != nil {
			scope.Log("upload_transactions: %v", err)
			return
		}

		for _, transaction := range outstanding {
			select {
			case <-ctx.Done():
				return
			case output_chan <- json.ConvertProtoToOrderedDict(transaction):
			}
		}

	}()

	return output_chan
}

func (self UploadTransactionsPlugin) Info(scope vfilter.Scope, type_map *vfilter.TypeMap) *vfilter.PluginInfo {
	return &vfilter.PluginInfo{
		Name:     "upload_transactions",
		Doc:      "View the outstanding transactions for uploads.",
		ArgType:  type_map.AddType(scope, &UploadTransactionsPluginArgs{}),
		Metadata: vql.VQLMetadata().Permissions(acls.READ_RESULTS).Build(),
	}
}

func init() {
	vql_subsystem.RegisterPlugin(&UploadTransactionsPlugin{})
}
