package launcher

import (
	"context"
	"sort"

	actions_proto "www.velocidex.com/golang/velociraptor/actions/proto"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	crypto_proto "www.velocidex.com/golang/velociraptor/crypto/proto"
	"www.velocidex.com/golang/velociraptor/file_store"
	"www.velocidex.com/golang/velociraptor/json"
	"www.velocidex.com/golang/velociraptor/paths"
	"www.velocidex.com/golang/velociraptor/result_sets"
	"www.velocidex.com/golang/velociraptor/services"
	"www.velocidex.com/golang/velociraptor/utils"
)

func (self *Launcher) ResumeFlow(
	ctx context.Context,
	config_obj *config_proto.Config,
	client_id, flow_id string) ([]*actions_proto.UploadTransaction, error) {

	file_store_factory := file_store.GetFileStore(config_obj)

	flow_path_manager := paths.NewFlowPathManager(client_id, flow_id)
	rs_reader, err := result_sets.NewResultSetReader(
		file_store_factory, flow_path_manager.UploadTransactions())
	if err != nil {
		return nil, err
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

	collection_context, err := self.Storage().LoadCollectionContext(
		ctx, config_obj,
		client_id, flow_id)
	if err != nil {
		return nil, err
	}

	request := &crypto_proto.VeloMessage{
		SessionId: flow_id,
		ResumeTransactions: &crypto_proto.ResumeTransactions{
			FlowId:     flow_id,
			ClientId:   client_id,
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
		return nil, err
	}

	err = client_info_manager.QueueMessageForClient(ctx, client_id,
		request, services.NOTIFY_CLIENT, utils.BackgroundWriter)
	if err != nil {
		return nil, err
	}

	var transactions []*actions_proto.UploadTransaction
	for _, t := range outstanding {
		transactions = append(transactions, t)
	}

	sort.Slice(transactions, func(i, j int) bool {
		return transactions[i].UploadId < transactions[j].UploadId
	})

	return transactions, nil
}
