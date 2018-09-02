package flows

import (
	"context"
	"encoding/json"
	"time"

	"github.com/golang/protobuf/proto"
	"github.com/golang/protobuf/ptypes"
	errors "github.com/pkg/errors"
	actions_proto "www.velocidex.com/golang/velociraptor/actions/proto"
	api_proto "www.velocidex.com/golang/velociraptor/api/proto"
	config "www.velocidex.com/golang/velociraptor/config"
	crypto_proto "www.velocidex.com/golang/velociraptor/crypto/proto"
	datastore "www.velocidex.com/golang/velociraptor/datastore"
	flows_proto "www.velocidex.com/golang/velociraptor/flows/proto"
	"www.velocidex.com/golang/velociraptor/grpc_client"
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

func (self *CheckHuntCondition) New() Flow {
	return &CheckHuntCondition{&BaseFlow{}}
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

	// Notify the client that it has new messages now.
	channel := grpc_client.GetChannel(config_obj)
	defer channel.Close()

	client := api_proto.NewAPIClient(channel)
	client.NotifyClients(context.Background(),
		&api_proto.NotificationRequest{
			ClientId: flow_obj.RunnerArgs.ClientId,
		})

	return nil
}

func (self *CheckHuntCondition) ProcessMessage(
	config_obj *config.Config,
	flow_obj *AFF4FlowObject,
	message *crypto_proto.GrrMessage) error {

	switch message.RequestId {
	case processConditionQuery:
		err := flow_obj.FailIfError(config_obj, message)
		if err != nil {
			return err
		}

		if flow_obj.IsRequestComplete(message) {
			return flow_obj.Complete(config_obj)
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

	// Make a stored query from the results of the first
	// query. This can now be queries on again server side.
	stored_query, err := _NewStoredQuery(vql_response)
	if err != nil {
		return false, err
	}

	// The query is empty dont bother matching.
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
	env := vfilter.NewDict().Set("rows", stored_query)
	for _, item := range server_side_condition_query.Env {
		env.Set(item.Key, item.Value)
	}

	scope := vfilter.NewScope().AppendVars(env)

	rule_matched := false
	for _, query := range server_side_condition_query.Query {
		_ = query
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
	rows []*vfilter.Dict
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
	if len(self.rows) >= 1 {
		for k, _ := range *self.rows[0].ToDict() {
			result = append(result, k)
		}
	}
	return &result
}

func _NewStoredQuery(vql_response *actions_proto.VQLResponse) (*_StoredQuery, error) {
	result := &_StoredQuery{}
	var rows []map[string]interface{}
	err := json.Unmarshal([]byte(vql_response.Response), &rows)
	if err != nil {
		return nil, errors.WithStack(err)
	}

	for _, row := range rows {
		item := vfilter.NewDict()
		for k, v := range row {
			item.Set(k, v)
		}
		result.rows = append(result.rows, item)
	}

	return result, nil
}

func _getDefaultCollectorArgs() *actions_proto.VQLCollectorArgs {
	return &actions_proto.VQLCollectorArgs{
		Query: []*actions_proto.VQLRequest{
			&actions_proto.VQLRequest{
				Name: "Collect default client info",
				VQL: "SELECT OS, Architecture, Fqdn, Platform, " +
					"config.Client_labels from info()",
			},
		},
	}
}

func _CalculateFlowConditionQuery(in *api_proto.HuntCondition) (
	*actions_proto.VQLCollectorArgs, error) {

	generic_condition := in.GetGenericCondition()
	if generic_condition != nil {
		return generic_condition.FlowConditionQuery, nil
	}

	return _getDefaultCollectorArgs(), nil
}

func _CalculateServerSideConditionQuery(in *api_proto.HuntCondition) (
	*actions_proto.VQLCollectorArgs, error) {
	generic_condition := in.GetGenericCondition()
	if generic_condition != nil {
		return generic_condition.ServerSideConditionQuery, nil
	}

	os_condition := in.GetOs()
	if os_condition != nil {
		host_expr := ""
		switch os_condition.Os {
		case api_proto.HuntOsCondition_WINDOWS:
			host_expr = "windows"
		case api_proto.HuntOsCondition_LINUX:
			host_expr = "linux"
		case api_proto.HuntOsCondition_OSX:
			host_expr = "darwin"
		default:
			host_expr = "."
		}

		result := &actions_proto.VQLCollectorArgs{
			Env: []*actions_proto.VQLEnv{
				&actions_proto.VQLEnv{
					Key:   "_OS",
					Value: host_expr,
				},
			},
			Query: []*actions_proto.VQLRequest{
				&actions_proto.VQLRequest{
					Name: "Check OS match",
					VQL:  "SELECT OS from rows where OS =~ _OS",
				},
			},
		}

		return result, nil
	}

	return &actions_proto.VQLCollectorArgs{
		Query: []*actions_proto.VQLRequest{
			&actions_proto.VQLRequest{
				VQL: "SELECT * from rows",
			},
		},
	}, nil
}
