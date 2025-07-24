package flows

import (
	"context"

	"github.com/Velocidex/ordereddict"
	"www.velocidex.com/golang/velociraptor/acls"
	actions_proto "www.velocidex.com/golang/velociraptor/actions/proto"
	crypto_proto "www.velocidex.com/golang/velociraptor/crypto/proto"
	"www.velocidex.com/golang/velociraptor/file_store"
	"www.velocidex.com/golang/velociraptor/json"
	"www.velocidex.com/golang/velociraptor/paths"
	"www.velocidex.com/golang/velociraptor/result_sets"
	"www.velocidex.com/golang/velociraptor/services"
	"www.velocidex.com/golang/velociraptor/utils"
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

		file_store_factory := file_store.GetFileStore(config_obj)

		flow_path_manager := paths.NewFlowPathManager(arg.ClientId, arg.FlowId)
		rs_reader, err := result_sets.NewResultSetReader(
			file_store_factory, flow_path_manager.UploadTransactions())
		if err != nil {
			scope.Log("upload_transactions: %v", err)
			return
		}

		defer rs_reader.Close()

		outstanding := make(map[int64]*actions_proto.UploadTransaction)

		for row := range rs_reader.Rows(ctx) {
			serialized, err := json.Marshal(row)
			if err != nil {
				continue
			}

			transaction := &actions_proto.UploadTransaction{}
			err = json.Unmarshal(serialized, transaction)
			if err != nil {
				scope.Log("upload_transactions: %v", err)
				continue
			}

			outstanding[transaction.UploadId] = transaction
			if transaction.Response != "" {
				delete(outstanding, transaction.UploadId)
			}

			file_path_manager := flow_path_manager.GetUploadsFile(
				transaction.Accessor, transaction.StoreAsName,
				transaction.Components)

			stat, err := file_store_factory.StatFile(file_path_manager.Path())
			if err == nil {
				transaction.StartOffset = stat.Size()
			}
		}

		launcher, err := services.GetLauncher(config_obj)
		if err != nil {
			scope.Log("upload_transactions: %v", err)
			return
		}

		collection_context, err := launcher.Storage().LoadCollectionContext(ctx, config_obj,
			arg.ClientId, arg.FlowId)
		if err != nil {
			scope.Log("upload_transactions: %v", err)
			return
		}

		request := &crypto_proto.VeloMessage{
			SessionId: arg.FlowId,
			ResumeTransactions: &crypto_proto.ResumeTransactions{
				FlowId:     arg.FlowId,
				ClientId:   arg.ClientId,
				Timeout:    collection_context.Request.Timeout,
				QueryStats: collection_context.QueryStats,
			},
		}
		for _, t := range outstanding {
			request.ResumeTransactions.Transactions = append(
				request.ResumeTransactions.Transactions, t)
		}

		client_info_manager, err := services.GetClientInfoManager(config_obj)
		if err != nil {
			scope.Log("upload_transactions: %v", err)
			return
		}

		json.Dump(request)

		err = client_info_manager.QueueMessageForClient(ctx, arg.ClientId,
			request, services.NOTIFY_CLIENT, utils.BackgroundWriter)
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
