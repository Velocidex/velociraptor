//
package flows

import (
	"encoding/json"

	"github.com/golang/protobuf/proto"
	"github.com/golang/protobuf/ptypes"
	errors "github.com/pkg/errors"
	actions_proto "www.velocidex.com/golang/velociraptor/actions/proto"
	"www.velocidex.com/golang/velociraptor/config"
	constants "www.velocidex.com/golang/velociraptor/constants"
	crypto_proto "www.velocidex.com/golang/velociraptor/crypto/proto"
	"www.velocidex.com/golang/velociraptor/datastore"
	flows_proto "www.velocidex.com/golang/velociraptor/flows/proto"
	"www.velocidex.com/golang/velociraptor/responder"
	"www.velocidex.com/golang/velociraptor/vql"
)

const (
	processClientInfo  uint64 = 1
	processCustomQuery uint64 = 2
)

type VInterrogate struct {
	BaseFlow
}

func (self *VInterrogate) New() Flow {
	return &VInterrogate{BaseFlow{}}
}

func (self *VInterrogate) Start(
	config_obj *config.Config,
	flow_obj *AFF4FlowObject,
	args proto.Message) error {
	interrogate_args, ok := args.(*flows_proto.VInterrogateArgs)
	if !ok {
		return errors.New("Expected args of type VInterrogateArgs")
	}

	// Run custom queries from the config file if present.
	if config_obj.Flows.InterrogateAdditionalQueries != nil {
		err := QueueMessageForClient(
			config_obj, flow_obj,
			"VQLClientAction",
			config_obj.Flows.InterrogateAdditionalQueries,
			processCustomQuery)
		if err != nil {
			return err
		}
	}

	// Run standard queries.
	queries := []*actions_proto.VQLRequest{
		&actions_proto.VQLRequest{
			VQL:  "select Version.Name, Version.BuildTime, Client.Labels from config",
			Name: "Client Info"},
		&actions_proto.VQLRequest{
			VQL: "select Hostname, OS, Architecture, Platform, PlatformVersion, " +
				"KernelVersion, Fqdn from info()",
			Name: "System Info"},
		&actions_proto.VQLRequest{
			VQL: "select ut_type, ut_id, ut_host as Host, " +
				"ut_user as User, " +
				"timestamp(epoch=ut_tv.tv_sec) as login_time from " +
				"users() where ut_type =~ 'USER'",
			Name: "Recent Users"},
	}

	for _, query := range interrogate_args.Queries {
		if query.VQL != "" {
			queries = append(queries, query)
		}
	}

	vql_request := &actions_proto.VQLCollectorArgs{
		Query: queries,
	}

	err := QueueMessageForClient(
		config_obj, flow_obj,
		"VQLClientAction",
		vql_request, processClientInfo)
	if err != nil {
		return err
	}

	flow_obj.SetState(&actions_proto.ClientInfo{})

	return nil
}

func (self *VInterrogate) ProcessMessage(
	config_obj *config.Config,
	flow_obj *AFF4FlowObject,
	message *crypto_proto.GrrMessage) error {

	switch message.RequestId {
	case processCustomQuery:
		client_info := flow_obj.GetState().(*actions_proto.ClientInfo)
		defer flow_obj.SetState(client_info)

		vql_response, ok := responder.ExtractGrrMessagePayload(
			message).(*actions_proto.VQLResponse)
		if ok {
			client_info.Info = append(client_info.Info, vql_response)
		}

	case processClientInfo:
		err := flow_obj.FailIfError(config_obj, message)
		if err != nil {
			return err
		}

		if flow_obj.IsRequestComplete(message) {
			// The flow is complete - store the client
			// info from our state into the client's AFF4
			// object.
			err := self.StoreClientInfo(config_obj, flow_obj)
			if err != nil {
				return err
			}
			return flow_obj.Complete(config_obj)
		}

		// Retrieve the client info from the flow state and
		// modify it by adding the response to it.
		client_info := flow_obj.GetState().(*actions_proto.ClientInfo)
		defer flow_obj.SetState(client_info)

		vql_response, ok := responder.ExtractGrrMessagePayload(
			message).(*actions_proto.VQLResponse)
		if ok {
			client_info.Info = append(client_info.Info, vql_response)
			switch vql_response.Query.Name {
			case "System Info":
				err := processSystemInfo(vql_response, client_info)
				if err != nil {
					return err
				}
			case "Client Info":
				processClientInfoQuery(vql_response, client_info)
			case "Recent Users":
				processRecentUsers(vql_response, client_info)
			}
		}

		// Also support receiving files in interrogate
		// actions.
	case constants.TransferWellKnownFlowId:
		delegate := &VQLCollector{}
		err := delegate.ProcessMessage(config_obj, flow_obj, message)
		if err != nil {
			return err
		}
	}

	return nil
}

func (self *VInterrogate) StoreClientInfo(
	config_obj *config.Config,
	flow_obj *AFF4FlowObject) error {

	client_info := flow_obj.GetState().(*actions_proto.ClientInfo)
	client_urn := "aff4:/" + flow_obj.RunnerArgs.ClientId
	db, err := datastore.GetDB(config_obj)
	if err != nil {
		return err
	}

	err = db.SetSubject(config_obj, client_urn, client_info)
	if err != nil {
		return err
	}

	// Update the client index for the GUI. Add any keywords we
	// wish to be searchable in the UI here.
	keywords := []string{
		"all", // This is used for "." search
		flow_obj.RunnerArgs.ClientId,
		client_info.Hostname,
		client_info.Fqdn,
		"host:" + client_info.Hostname,
	}

	if client_info.Knowledgebase != nil {
		for _, user := range client_info.Knowledgebase.Users {
			keywords = append(keywords, "user:"+user.Username)
			keywords = append(keywords, user.Username)
		}
	}

	return db.SetIndex(config_obj,
		constants.CLIENT_INDEX_URN,
		flow_obj.RunnerArgs.ClientId,
		keywords,
	)
}

type parseRecentUsers struct {
	User string
}

func processRecentUsers(response *actions_proto.VQLResponse,
	client_info *actions_proto.ClientInfo) error {
	var result []parseRecentUsers
	users := make(map[string]bool)

	err := json.Unmarshal([]byte(response.Response), &result)
	if err != nil {
		return errors.WithStack(err)
	}

	client_info.Knowledgebase = &actions_proto.Knowledgebase{}
	for _, info := range result {
		users[info.User] = true
	}

	user_string := ""
	for k := range users {
		user_string += " " + k
		user := &actions_proto.User{
			Username: k,
		}
		client_info.Knowledgebase.Users = append(
			client_info.Knowledgebase.Users, user)
	}

	client_info.Usernames = user_string
	return nil
}

func processSystemInfo(response *actions_proto.VQLResponse,
	client_info *actions_proto.ClientInfo) error {
	var result []vql.InfoStat

	err := json.Unmarshal([]byte(response.Response), &result)
	if err != nil {
		return errors.WithStack(err)
	}
	for _, info := range result {
		client_info.Hostname = info.Hostname
		client_info.System = info.OS
		client_info.Release = info.Platform + info.PlatformVersion
		client_info.Architecture = info.Architecture
		client_info.Fqdn = info.Fqdn
	}
	return nil
}

func processClientInfoQuery(response *actions_proto.VQLResponse,
	client_info *actions_proto.ClientInfo) error {
	var result []map[string]string

	err := json.Unmarshal([]byte(response.Response), &result)
	if err != nil {
		return errors.WithStack(err)
	}

	for _, info := range result {
		if info != nil {
			client_info.ClientName = info["Name"]
			client_info.ClientVersion = info["BuildTime"]
		}
	}
	return nil
}

func init() {
	impl := VInterrogate{}
	default_args, _ := ptypes.MarshalAny(&flows_proto.VInterrogateArgs{})
	desc := &flows_proto.FlowDescriptor{
		Name:         "VInterrogate",
		FriendlyName: "Client Interrogate",
		Category:     "Administrative",
		Doc:          "Discover basic facts about the client's system.",
		ArgsType:     "VInterrogateArgs",
		DefaultArgs:  default_args,
	}
	RegisterImplementation(desc, &impl)
}
