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

import (
	"errors"
	"regexp"
)

const (
	VERSION                    = "0.5.1"
	ENROLLMENT_WELL_KNOWN_FLOW = "E:Enrol"
	MONITORING_WELL_KNOWN_FLOW = FLOW_PREFIX + "Monitoring"

	FLOW_PREFIX             = "F."
	FOREMAN_WELL_KNOWN_FLOW = "E.Foreman"
	HUNT_PREFIX             = "H."

	// The GUI uses this as the client index.
	CLIENT_INDEX_URN = "/client_index/"

	// An index of all the hunts and clients.
	HUNT_INDEX     = "/hunt_index/"
	NOTEBOOK_INDEX = "/notebook_index/"
	USER_URN       = "/users/"

	// Well known flows - Request ID:
	LOG_SINK uint64 = 980

	TransferWellKnownFlowId = 5
	ProcessVQLResponses     = 1

	// Largest buffer we use for comms.
	MAX_MEMORY    = 5 * 1024 * 1024
	MAX_POST_SIZE = 5 * 1024 * 1024

	// Filestore paths for artifacts must begin with this prefix.
	ARTIFACT_DEFINITION_PREFIX = "/artifact_definitions/"

	// Messages to the client which we dont care about their responses.
	IgnoreResponseState = uint64(101)

	// These store configuration for the server and client
	// monitoring artifacts.
	ServerMonitoringFlowURN = "/config/server_monitoring.json"
	ClientMonitoringFlowURN = "/config/client_monitoring.json"
	ThirdPartyInventory     = "/config/inventory.json"

	USER_AGENT = "Velociraptor - Dig Deeper!"

	// Internal artifact names.
	CLIENT_INFO_ARTIFACT = "Generic.Client.Info"

	// Globals set in VQL scopes.
	SCOPE_CONFIG        = "config"
	SCOPE_SERVER_CONFIG = "server_config"
	SCOPE_THROTTLE      = "$throttle"
	SCOPE_UPLOADER      = "$uploader"
	SCOPE_RESPONDER     = "$responder"
	SCOPE_MOCK          = "$mock"
	SCOPE_ROOT          = "$root"
	SCOPE_STACK         = "$stack"

	// Artifact names from packs should start with this
	ARTIFACT_PACK_NAME_PREFIX   = "Packs."
	ARTIFACT_CUSTOM_NAME_PREFIX = "Custom."

	// USER record encoded in grpc context
	GRPC_USER_CONTEXT key = iota
)

type key int

var (
	HuntIdRegex    = regexp.MustCompile(`^H\.[^.]+$`)
	STOP_ITERATION = errors.New("Stop Iteration")
)
