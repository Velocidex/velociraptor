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
package constants

import "regexp"

const (
	VERSION                    = "0.3.0"
	ENROLLMENT_WELL_KNOWN_FLOW = "aff4:/flows/E:Enrol"
	MONITORING_WELL_KNOWN_FLOW = FLOW_PREFIX + "Monitoring"

	// Temporary attribute
	AFF4_ATTR = "aff4:data"

	FLOW_PREFIX             = "F."
	FOREMAN_WELL_KNOWN_FLOW = "aff4:/flows/E.Foreman"
	HUNTS_URN               = "aff4:/hunts/"
	HUNT_PREFIX             = "H."

	// The GUI uses this as the client index.
	CLIENT_INDEX_URN = "aff4:/client_index/"

	USER_URN = "aff4:/users/"

	// Well known flows - Request ID:
	LOG_SINK uint64 = 980

	TransferWellKnownFlowId = 5

	// Largest buffer we use for comms.
	MAX_MEMORY    = 5 * 1024 * 1024
	MAX_POST_SIZE = 5 * 1024 * 1024

	// Filestore paths for artifacts must begin with this prefix.
	ARTIFACT_DEFINITION_PREFIX = "/artifact_definitions/"

	// Messages to the client which we dont care about their responses.
	IgnoreResponseState = uint64(101)

	FRONTEND_NAME       = "VelociraptorServer"
	GRPC_GW_CLIENT_NAME = "GRPC_GW"

	// These store configuration for the server and client
	// monitoring artifacts.
	ServerMonitoringFlowURN = "aff4:/config/server_monitoring.json"
	ClientMonitoringFlowURN = "aff4:/config/client_monitoring.json"
)

var (
	HuntIdRegex = regexp.MustCompile(`^H\.[^.]+$`)
)
