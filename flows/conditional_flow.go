package flows

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/golang/protobuf/proto"
	"github.com/golang/protobuf/ptypes"
	"time"
	actions_proto "www.velocidex.com/golang/velociraptor/actions/proto"
	api_proto "www.velocidex.com/golang/velociraptor/api/proto"
	config "www.velocidex.com/golang/velociraptor/config"
	constants "www.velocidex.com/golang/velociraptor/constants"
	crypto_proto "www.velocidex.com/golang/velociraptor/crypto/proto"
	datastore "www.velocidex.com/golang/velociraptor/datastore"
	flows_proto "www.velocidex.com/golang/velociraptor/flows/proto"
	"www.velocidex.com/golang/velociraptor/responder"
	utils "www.velocidex.com/golang/velociraptor/testing"
	"www.velocidex.com/golang/vfilter"
)

var (
	processConditionQuery uint64 = 100
	processUpgradeForeman uint64 = 101
)

type ConditionalFlow struct {
	*BaseFlow
}

func (self *ConditionalFlow) Start(
	config_obj *config.Config,
	flow_obj *AFF4FlowObject,
	args proto.Message) (*string, error) {
	hunt_args, ok := args.(*api_proto.Hunt)
	if !ok {
		return nil, errors.New("Expected args of type Hunt")
	}

	flow_id := GetNewFlowIdForClient(flow_obj.RunnerArgs.ClientId)
	err := QueueMessageForClient(
		config_obj, flow_obj.RunnerArgs.ClientId,
		flow_id,
		"VQLClientAction",
		hunt_args.FlowConditionQuery, processConditionQuery)
	if err != nil {
		return nil, err
	}

	err = QueueMessageForClient(
		config_obj, flow_obj.RunnerArgs.ClientId,
		flow_id,
		"UpdateForeman",
		&actions_proto.ForemanCheckin{
			LastHuntTimestamp: hunt_args.CreateTime,
		}, processUpgradeForeman)
	if err != nil {
		return nil, err
	}

	return &flow_id, nil
}

func (self *ConditionalFlow) ProcessMessage(
	config_obj *config.Config,
	flow_obj *AFF4FlowObject,
	message *crypto_proto.GrrMessage) error {

	switch message.RequestId {
	case processConditionQuery:
		err := flow_obj.FailIfError(message)
		if err != nil {
			return err
		}

		if flow_obj.IsRequestComplete(message) {
			flow_obj.Complete()
			return nil
		}

		hunt, err := _ExtractHuntArgs(flow_obj)
		if err != nil {
			return err
		}
		condition_applied, err := _FilterConditionServerSide(
			flow_obj, hunt, message)
		if err != nil {
			return err
		}

		utils.Debug(condition_applied)
		if condition_applied {
			// Write the hunt in the pending queue.
			info_urn := hunt.HuntId + "/pending/" + message.Source
			info := &api_proto.HuntInfo{
				HuntId:        hunt.HuntId,
				ScheduledTime: uint64(time.Now().UnixNano() / 1000),
				ClientId:      message.Source,
				StartRequest:  hunt.StartRequest,
				State:         api_proto.HuntInfo_PENDING,
			}
			serialized_info, err := proto.Marshal(info)
			if err != nil {
				return err
			}

			data := make(map[string][]byte)
			data[constants.HUNTS_SUMMARY_ATTR] = serialized_info
			db, err := datastore.GetDB(config_obj)
			if err != nil {
				return err
			}

			err = db.SetSubjectData(config_obj, info_urn, 0, data)
			if err != nil {
				return err
			}
		}
	}

	return nil
}

func _ExtractHuntArgs(flow_obj *AFF4FlowObject) (*api_proto.Hunt, error) {
	flow_args, err := GetFlowArgs(flow_obj.RunnerArgs)
	if err != nil {
		return nil, err
	}

	args, ok := flow_args.(*api_proto.Hunt)
	if !ok {
		return nil, errors.New("Expected args of type Hunt")
	}

	return args, nil
}

func _FilterConditionServerSide(
	flow_obj *AFF4FlowObject,
	args *api_proto.Hunt,
	message *crypto_proto.GrrMessage) (bool, error) {

	vql_response, ok := responder.ExtractGrrMessagePayload(
		message).(*actions_proto.VQLResponse)
	if !ok {
		return false, errors.New("Expected VQLResponse")
	}

	var rows []map[string]interface{}
	err := json.Unmarshal([]byte(vql_response.Response), &rows)
	if err != nil {
		return false, err
	}
	stored_query := &_StoredQuery{rows: rows}
	if len(stored_query.rows) == 0 {
		return false, nil
	}

	if len(args.ServerSideConditionQuery.Query) == 0 {
		return true, nil
	}

	// Run the server side query.
	rule_matched := false
	scope := vfilter.NewScope().AppendVars(
		vfilter.NewDict().Set("rows", stored_query))
	for _, query := range args.ServerSideConditionQuery.Query {
		vql, err := vfilter.Parse(query.VQL)
		if err != nil {
			return false, err
		}

		ctx := context.Background()
		row_chan := vql.Eval(ctx, scope)
		for _ = range row_chan {
			rule_matched = true
		}

		return rule_matched, nil
	}

	return true, nil
}

func init() {
	impl := ConditionalFlow{}
	default_args, _ := ptypes.MarshalAny(&api_proto.Hunt{})
	desc := &flows_proto.FlowDescriptor{
		Name:         "ConditionalFlow",
		FriendlyName: "Conditional Flow",
		Category:     "Collectors",
		Doc:          "Runs a delegate flow based on VQL conditions.",
		ArgsType:     "Hunt",
		DefaultArgs:  default_args,
	}

	RegisterImplementation(desc, &impl)
}

type _StoredQuery struct {
	rows []map[string]interface{}
}

func (self *_StoredQuery) Eval(ctx context.Context) <-chan vfilter.Row {
	result := make(chan vfilter.Row)

	go func() {
		defer close(result)
		for _, row := range self.rows {
			result <- row
		}
	}()

	return result
}

func (self *_StoredQuery) Columns() *[]string {
	result := []string{}
	if len(self.rows) > 1 {
		for k, _ := range self.rows[0] {
			result = append(result, k)
		}
	}
	return &result
}
