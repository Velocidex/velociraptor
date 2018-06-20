//
package flows

import (
	"encoding/json"
	"github.com/golang/protobuf/proto"

	"errors"
	actions_proto "www.velocidex.com/golang/velociraptor/actions/proto"
	"www.velocidex.com/golang/velociraptor/config"
	"www.velocidex.com/golang/velociraptor/constants"
	crypto_proto "www.velocidex.com/golang/velociraptor/crypto/proto"
	"www.velocidex.com/golang/velociraptor/datastore"
	flows_proto "www.velocidex.com/golang/velociraptor/flows/proto"
	"www.velocidex.com/golang/velociraptor/responder"
	utils "www.velocidex.com/golang/velociraptor/testing"
	"www.velocidex.com/golang/velociraptor/vql"
)

const (
	_                        = iota
	processClientInfo uint64 = 1
)

type VInterrogate struct{}

func (self *VInterrogate) Start(
	config_obj *config.Config,
	flow_obj *AFF4FlowObject,
	args proto.Message) (*string, error) {
	interrogate_args, ok := args.(*flows_proto.VInterrogateArgs)
	if !ok {
		return nil, errors.New("Expected args of type VInterrogateArgs")
	}

	db, err := datastore.GetDB(config_obj)
	if err != nil {
		return nil, err
	}

	flow_id := GetNewFlowIdForClient(flow_obj.Runner_args.ClientId)
	queries := []*actions_proto.VQLRequest{
		&actions_proto.VQLRequest{
			VQL:  "select Client_name, Client_build_time, Client_labels from config",
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
		queries = append(queries, query)
	}

	vql_request := &actions_proto.VQLCollectorArgs{
		Query: queries,
	}

	err = db.QueueMessageForClient(
		config_obj, flow_obj.Runner_args.ClientId,
		flow_id,
		"VQLClientAction",
		vql_request, processClientInfo)
	if err != nil {
		return nil, err
	}

	flow_obj.SetState(&actions_proto.ClientInfo{})

	return &flow_id, nil
}

func (self *VInterrogate) ProcessMessage(
	config_obj *config.Config,
	flow_obj *AFF4FlowObject,
	message *crypto_proto.GrrMessage) error {

	switch message.RequestId {
	case processClientInfo:
		err := flow_obj.FailIfError(message)
		if err != nil {
			return err
		}

		if flow_obj.IsRequestComplete(message) {
			defer flow_obj.Complete()

			// The flow is complete - store the client
			// info from our state into the client's AFF4
			// object.
			err := self.StoreClientInfo(config_obj, flow_obj)
			if err != nil {
				return err
			}
			return nil
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
	}

	return nil
}

func (self *VInterrogate) StoreClientInfo(
	config_obj *config.Config,
	flow_obj *AFF4FlowObject) error {

	client_info := flow_obj.GetState().(*actions_proto.ClientInfo)

	client_urn := "aff4:/" + flow_obj.Runner_args.ClientId
	db, err := datastore.GetDB(config_obj)
	if err != nil {
		return err
	}

	data := make(map[string][]byte)
	encoded_client_info, err := proto.Marshal(client_info)
	if err != nil {
		return err
	}

	// FIXME: GRR spreads this information over multiple
	// attributes. When we refactor the GRR API handlers remove
	// these attributes, and just read them all from the
	// ClientInfo proto.
	data[constants.AFF4_TYPE] = []byte("VFSGRRClient")
	data[constants.CLIENT_VELOCIRAPTOR_INFO] = encoded_client_info
	data["metadata:hostname"] = []byte(client_info.Hostname)
	data["metadata:fqdn"] = []byte(client_info.Fqdn)
	data["metadata:system"] = []byte(client_info.System)
	data["metadata:os_version"] = []byte(client_info.System)
	data["metadata:architecture"] = []byte(client_info.Architecture)
	data["aff4:user_names"] = []byte(client_info.Usernames)
	client_information := &actions_proto.ClientInformation{
		ClientName: client_info.ClientName,
		BuildTime:  client_info.ClientVersion,
	}
	serialized_client_info, err := proto.Marshal(client_information)
	if err != nil {
		return err
	}
	data["metadata:ClientInfo"] = serialized_client_info

	serialized_kb, err := proto.Marshal(client_info.Knowledgebase)
	if err != nil {
		return err
	}
	data["metadata:knowledge_base"] = serialized_kb

	err = db.SetSubjectData(config_obj, client_urn, datastore.LatestTime, data)
	if err != nil {
		return err
	}

	// Update the client index for the GUI. Add any keywords we
	// wish to be searchable in the UI here.
	keywords := []string{
		"", // This is used for "." search
		flow_obj.Runner_args.ClientId,
		client_info.Hostname,
		client_info.Fqdn,
		"host:" + client_info.Hostname,
	}

	for _, user := range client_info.Knowledgebase.Users {
		keywords = append(keywords, "user:"+user.Username)
		keywords = append(keywords, user.Username)
	}

	err = db.SetIndex(config_obj,
		constants.CLIENT_INDEX_URN,
		flow_obj.Runner_args.ClientId,
		keywords,
	)
	if err != nil {
		return err
	}
	return nil
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
		utils.Debug(err)
		return err
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
		return err
	}
	for _, info := range result {
		client_info.Hostname = info.Hostname
		client_info.System = info.OS
		client_info.Release = info.Platform + info.PlatformVersion
		client_info.Architecture = info.Architecture
	}
	return nil
}

func processClientInfoQuery(response *actions_proto.VQLResponse,
	client_info *actions_proto.ClientInfo) error {
	var result []config.Config

	err := json.Unmarshal([]byte(response.Response), &result)
	if err != nil {
		return err
	}

	for _, info := range result {
		if info.Client_name == nil || info.Client_build_time == nil {
			continue
		}
		client_info.ClientName = *info.Client_name
		client_info.ClientVersion = *info.Client_build_time
	}
	return nil
}

func init() {
	impl := VInterrogate{}
	RegisterImplementation("VInterrogate", &impl)
}
