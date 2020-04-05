/*
   Velociraptor - Hunting Evil
   Copyright (C) 2019 Velocidex Innovations.

   This program is free software: you can redistribute it and/or modify
   it under the terms of the GNU Affero General Public License as published
   by the Free Software Foundation, either version 3 of the License, or
   (at your option) any later version.

   This program is distributed in the hope that it will be useful,
   but WITHOUT ANY WARRANTY; without even the implied warranty of
   MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
   GNU Affero General Public License for more details.

   You should have received a copy of the GNU Affero General Public License
   along with this program.  If not, see <https://www.gnu.org/licenses/>.
*/
package api

import (
	"errors"
	"net"
	"strings"
	"time"

	actions_proto "www.velocidex.com/golang/velociraptor/actions/proto"
	api_proto "www.velocidex.com/golang/velociraptor/api/proto"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	"www.velocidex.com/golang/velociraptor/constants"
	"www.velocidex.com/golang/velociraptor/datastore"
	"www.velocidex.com/golang/velociraptor/server"
	urns "www.velocidex.com/golang/velociraptor/urns"
)

func GetApiClient(
	config_obj *config_proto.Config,
	server_obj *server.Server,
	client_id string, detailed bool) (
	*api_proto.ApiClient, error) {

	result := &api_proto.ApiClient{
		ClientId: client_id,
	}

	// Special well know client id.
	if client_id == "server" {
		return result, nil
	}

	if client_id == "" || client_id[0] != 'C' {
		return nil, errors.New("client_id must start with C")
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

	result.LastInterrogateFlowId = client_info.LastInterrogateFlowId
	result.AgentInformation = &api_proto.AgentInformation{
		Version: client_info.ClientVersion,
		Name:    client_info.ClientName,
	}

	result.OsInfo = &api_proto.Uname{
		System:  client_info.System,
		Release: client_info.Release,
		Machine: client_info.Architecture,
		Fqdn:    client_info.Fqdn,
	}

	err = db.GetSubject(
		config_obj, urns.BuildURN("clients", client_id, "ping"),
		client_info)
	if err != nil {
		return nil, err
	}

	result.LastSeenAt = client_info.Ping
	result.LastIp = client_info.IpAddress

	// Update the time to now if the client is currently actually
	// connected.
	if server_obj != nil &&
		server_obj.NotificationPool.IsClientConnected(client_id) {
		result.LastSeenAt = uint64(time.Now().UnixNano() / 1000)
	}

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
	config_obj *config_proto.Config,
	in *api_proto.LabelClientsRequest) (*api_proto.APIResponse, error) {
	db, err := datastore.GetDB(config_obj)
	if err != nil {
		return nil, err
	}

	index_func := db.SetIndex
	switch in.Operation {
	case "remove":
		index_func = db.UnsetIndex
	case "check":
		index_func = db.CheckIndex
	case "set":
		// default.
	default:
		return nil, errors.New(
			"unknown label operation. Must be set, check or remove")
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
