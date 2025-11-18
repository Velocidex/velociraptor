package actions

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/Velocidex/ordereddict"
	actions_proto "www.velocidex.com/golang/velociraptor/actions/proto"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	"www.velocidex.com/golang/velociraptor/constants"
	crypto_proto "www.velocidex.com/golang/velociraptor/crypto/proto"
	"www.velocidex.com/golang/velociraptor/json"
	"www.velocidex.com/golang/velociraptor/responder"
	"www.velocidex.com/golang/velociraptor/services"
	"www.velocidex.com/golang/velociraptor/uploads"
	"www.velocidex.com/golang/velociraptor/utils"
	"www.velocidex.com/golang/velociraptor/vql/acl_managers"
)

// ResumeTransactions replays transactions through the client uploader
// in order to resume uploads. The results are added to the original
// flow, and any additional logs are also appended to the original
// flow.
func ResumeTransactions(
	ctx context.Context,
	config_obj *config_proto.Config,
	responder responder.Responder,
	stat *crypto_proto.VeloStatus,
	req *crypto_proto.ResumeTransactions) {

	defer responder.Return(ctx)

	timeout := req.Timeout
	if timeout == 0 {
		timeout = 600
	}

	manager, err := services.GetRepositoryManager(config_obj)
	if err != nil {
		responder.RaiseError(ctx, fmt.Sprintf("%v", err))
		return
	}

	repository := manager.NewRepository()

	logger := log.New(NewLogWriter(ctx, config_obj, responder), "", 0)
	uploader := uploads.NewVelociraptorUploader(ctx, logger,
		time.Duration(timeout)*time.Second, responder)
	defer uploader.Close()

	builder := services.ScopeBuilder{
		Config: &config_proto.Config{
			Client:     config_obj.Client,
			Remappings: config_obj.Remappings,
		},
		Ctx: ctx,

		// Only provide the client config since we are running in
		// client context.
		ClientConfig: config_obj.Client,
		// Disable ACLs on the client.
		ACLManager: acl_managers.NullACLManager{},
		Env: ordereddict.NewDict().
			// Make the session id available in the query.
			Set("_SessionId", responder.FlowContext().SessionId()).
			Set(constants.SCOPE_RESPONDER, responder),
		Uploader:   uploader,
		Repository: repository,
		Logger:     logger,
	}

	scope := manager.BuildScope(builder)
	defer scope.Close()

	// Uploader needs an active scope so it needs to close before the
	// scope is destroyed because it still needs to use transaction
	// scopes.
	defer uploader.Close()

	scope.Log("INFO:Resuming uploads: %v transactions.", len(req.Transactions))

	var rows []*ordereddict.Dict

	for _, t := range req.Transactions {
		row := ordereddict.NewDict().
			Set("ReplayTime", utils.GetTime().Now())
		row.MergeFrom(json.ConvertProtoToOrderedDict(t))
		rows = append(rows, row)

		uploader.ReplayTransaction(ctx, scope, t)
	}

	jsonl, err := json.MarshalJsonl(rows)
	if err == nil && len(rows) > 0 {
		response := &actions_proto.VQLResponse{
			Query: &actions_proto.VQLRequest{
				Name: constants.UPLOAD_RESUMED_SOURCE,
			},
			JSONLResponse: string(jsonl),
			TotalRows:     uint64(len(rows)),
			QueryStartRow: uint64(stat.ResultRows),
			Timestamp:     uint64(utils.Now().UTC().UnixNano() / 1000),
			Columns:       rows[0].Keys(),
		}
		responder.AddResponse(&crypto_proto.VeloMessage{
			VQLResponse: response})
	}

	// Wait here until the uploader is done.
	uploader.Close()
}
