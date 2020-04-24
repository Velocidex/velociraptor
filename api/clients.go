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
	crypto_proto "www.velocidex.com/golang/velociraptor/crypto/proto"
	"www.velocidex.com/golang/velociraptor/datastore"
	"www.velocidex.com/golang/velociraptor/paths"
	"www.velocidex.com/golang/velociraptor/server"
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

	client_urn := paths.GetClientMetadataPath(client_id)
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

	public_key_info := &crypto_proto.PublicKey{}
	err = db.GetSubject(config_obj, paths.GetClientKeyPath(client_id),
		public_key_info)
	if err != nil {
		return nil, err
	}

	result.FirstSeenAt = public_key_info.EnrollTime

	err = db.GetSubject(config_obj, paths.GetClientPingPath(client_id),
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
