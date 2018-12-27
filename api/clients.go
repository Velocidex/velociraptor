package api

import (
	"errors"
	"net"
	"strings"

	actions_proto "www.velocidex.com/golang/velociraptor/actions/proto"
	api_proto "www.velocidex.com/golang/velociraptor/api/proto"
	"www.velocidex.com/golang/velociraptor/constants"
	"www.velocidex.com/golang/velociraptor/datastore"
	urns "www.velocidex.com/golang/velociraptor/urns"
)

func GetApiClient(
	config_obj *api_proto.Config, client_id string, detailed bool) (
	*api_proto.ApiClient, error) {

	if client_id[0] != 'C' {
		return nil, errors.New("Client_id must start with C")
	}

	result := &api_proto.ApiClient{
		ClientId: client_id,
	}

	client_urn := urns.BuildURN("clients", client_id)
	db, err := datastore.GetDB(config_obj)
	if err != nil {
		return nil, err
	}

	for _, label := range db.SearchClients(
		config_obj, constants.CLIENT_INDEX_URN,
		client_id, "", 0, 1000) {
		if strings.HasPrefix(label, "label:") {
			result.Labels = append(
				result.Labels, strings.TrimPrefix(label, "label:"))
		}
	}

	client_info := &actions_proto.ClientInfo{}
	err = db.GetSubject(config_obj, client_urn, client_info)
	if err != nil {
		return nil, err
	}

	if detailed {
		result.Info = client_info.Info
	}

	result.AgentInformation = &api_proto.AgentInformation{
		Version: client_info.ClientVersion,
		Name:    client_info.ClientName,
	}

	result.OsInfo = &actions_proto.Uname{
		System:  client_info.System,
		Release: client_info.Release,
		Machine: client_info.Architecture,
		Fqdn:    client_info.Fqdn,
	}

	if client_info.Knowledgebase != nil {
		for _, user := range client_info.Knowledgebase.Users {
			result.Users = append(result.Users, user)
		}
	}

	err = db.GetSubject(
		config_obj,
		urns.BuildURN(client_urn, "ping"),
		client_info)
	if err != nil {
		return nil, err
	}

	result.LastSeenAt = client_info.Ping
	result.LastIp = client_info.IpAddress

	remote_address := strings.Split(result.LastIp, ":")[0]
	if _is_ip_in_ranges(remote_address, config_obj.GUI.InternalCidr) {
		result.LastIpClass = api_proto.ApiClient_INTERNAL
	} else if _is_ip_in_ranges(remote_address, config_obj.GUI.InternalCidr) {
		result.LastIpClass = api_proto.ApiClient_VPN
	} else {
		result.LastIpClass = api_proto.ApiClient_EXTERNAL
	}

	return result, nil
}

func LabelClients(
	config_obj *api_proto.Config,
	in *api_proto.LabelClientsRequest) (*api_proto.APIResponse, error) {
	db, err := datastore.GetDB(config_obj)
	if err != nil {
		return nil, err
	}

	index_func := db.SetIndex
	if in.Operation == "remove" {
		index_func = db.UnsetIndex
	}

	for _, label := range in.Labels {
		for _, client_id := range in.ClientIds {
			if !strings.HasPrefix(label, "label:") {
				label = "label:" + label
			}
			err = index_func(
				config_obj,
				constants.CLIENT_INDEX_URN,
				client_id, []string{label})
			if err != nil {
				return nil, err
			}
			err = index_func(
				config_obj,
				constants.CLIENT_INDEX_URN,
				label, []string{client_id})
			if err != nil {
				return nil, err
			}
		}
	}

	return &api_proto.APIResponse{}, nil
}

func _is_ip_in_ranges(remote string, ranges []string) bool {
	for _, ip_range := range ranges {
		_, ipNet, err := net.ParseCIDR(ip_range)
		if err != nil {
			return false
		}

		if ipNet.Contains(net.ParseIP(remote)) {
			return true
		}
	}

	return false
}
