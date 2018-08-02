package flows

import (
	"context"
	"encoding/json"
	"github.com/golang/protobuf/proto"
	"github.com/golang/protobuf/ptypes"
	errors "github.com/pkg/errors"
	"time"
	actions_proto "www.velocidex.com/golang/velociraptor/actions/proto"
	api_proto "www.velocidex.com/golang/velociraptor/api/proto"
	config "www.velocidex.com/golang/velociraptor/config"
	crypto_proto "www.velocidex.com/golang/velociraptor/crypto/proto"
	datastore "www.velocidex.com/golang/velociraptor/datastore"
	flows_proto "www.velocidex.com/golang/velociraptor/flows/proto"
	"www.velocidex.com/golang/velociraptor/responder"
	"www.velocidex.com/golang/vfilter"
)

var (
	processConditionQuery uint64 = 100
	processUpgradeForeman uint64 = 101
)

type CheckHuntCondition struct {
	*BaseFlow
}

func (self *CheckHuntCondition) Start(
	config_obj *config.Config,
	flow_obj *AFF4FlowObject,
	args proto.Message) error {
	hunt_args, ok := args.(*api_proto.Hunt)
	if !ok {
		return errors.New("Expected args of type Hunt")
	}

	flow_condition_query, err := _CalculateFlowConditionQuery(hunt_args.Condition)
	if err != nil {
		return err
	}

	err = QueueMessageForClient(
		config_obj, flow_obj,
		"VQLClientAction",
		flow_condition_query,
		processConditionQuery)
	if err != nil {
		return err
	}

	err = QueueMessageForClient(
		config_obj, flow_obj,
		"UpdateForeman",
		&actions_proto.ForemanCheckin{
			LastHuntTimestamp: hunt_args.CreateTime,
		}, processUpgradeForeman)
	if err != nil {
		return err
	}

	return nil
}

func (self *CheckHuntCondition) ProcessMessage(
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

		if condition_applied {
			flow_obj.Log("Condition matched. Queueing hunt on client " +
				message.Source)
			// Write the hunt in the pending queue.
			info_urn := hunt.HuntId + "/pending/" + message.Source
			info := &api_proto.HuntInfo{
				HuntId:        hunt.HuntId,
				ScheduledTime: uint64(time.Now().UnixNano() / 1000),
				ClientId:      message.Source,
				StartRequest:  hunt.StartRequest,
				State:         api_proto.HuntInfo_PENDING,
			}
			db, err := datastore.GetDB(config_obj)
			if err != nil {
				return err
			}
			err = db.SetSubject(config_obj, info_urn, info)
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
		return false, errors.WithStack(err)
	}
	stored_query := &_StoredQuery{rows: rows}
	if len(stored_query.rows) == 0 {
		return false, nil
	}

	server_side_condition_query, err := _CalculateServerSideConditionQuery(
		args.Condition)
	if err != nil {
		return false, err
	}

	if len(server_side_condition_query.Query) == 0 {
		return true, nil
	}

	// Run the server side query.
	rule_matched := false
	scope := vfilter.NewScope().AppendVars(
		vfilter.NewDict().Set("rows", stored_query))
	for _, query := range server_side_condition_query.Query {
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
	impl := CheckHuntCondition{}
	default_args, _ := ptypes.MarshalAny(&api_proto.Hunt{})
	desc := &flows_proto.FlowDescriptor{
		Name:         "CheckHuntCondition",
		FriendlyName: "Check Hunt Condition",
		Category:     "Internal",
		Doc:          "Checks a VQL condition on the client for hunt membership.",
		ArgsType:     "Hunt",
		DefaultArgs:  default_args,
		Internal:     true,
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

func _CalculateFlowConditionQuery(in *api_proto.HuntCondition) (
	*actions_proto.VQLCollectorArgs, error) {

	return in.GetGenericCondition().FlowConditionQuery, nil
}

func _CalculateServerSideConditionQuery(in *api_proto.HuntCondition) (
	*actions_proto.VQLCollectorArgs, error) {
	return in.GetGenericCondition().ServerSideConditionQuery, nil
}
