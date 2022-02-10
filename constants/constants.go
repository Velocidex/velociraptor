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
	VERSION                    = "0.6.4-dev"
	ENROLLMENT_WELL_KNOWN_FLOW = "E:Enrol"
	MONITORING_WELL_KNOWN_FLOW = FLOW_PREFIX + "Monitoring"

	FLOW_PREFIX             = "F."
	FOREMAN_WELL_KNOWN_FLOW = "E.Foreman"
	HUNT_PREFIX             = "H."

	// Well known flows - Request ID:
	LOG_SINK uint64 = 980

	TransferWellKnownFlowId = 5
	ProcessVQLResponses     = 1

	// Largest buffer we use for comms.
	MAX_MEMORY    = 5 * 1024 * 1024
	MAX_POST_SIZE = 5 * 1024 * 1024

	// Messages to the client which we dont care about their responses.
	IgnoreResponseState = uint64(101)

	USER_AGENT = "Velociraptor - Dig Deeper!"

	// Globals set in VQL scopes.
	SCOPE_CONFIG         = "config"
	SCOPE_SERVER_CONFIG  = "server_config"
	SCOPE_THROTTLE       = "$throttle"
	SCOPE_UPLOADER       = "$uploader"
	SCOPE_RESPONDER      = "$responder"
	SCOPE_MOCK           = "$mock"
	SCOPE_ROOT           = "$root"
	SCOPE_STACK          = "$stack"
	SCOPE_DEVICE_MANAGER = "$device_manager"

	// Artifact names from packs should start with this
	ARTIFACT_PACK_NAME_PREFIX   = "Packs."
	ARTIFACT_CUSTOM_NAME_PREFIX = "Custom."

	// USER record encoded in grpc context
	GRPC_USER_CONTEXT key = iota

	// Configuration for VQL plugins. These can be set in an
	// artifact to control the way VQL works.

	// How often to expire the ntfs cache.
	NTFS_CACHE_TIME = "NTFS_CACHE_TIME"

	// Number of clusters to cache in memory (default 100).
	NTFS_CACHE_SIZE = "NTFS_CACHE_SIZE"

	RAW_REG_CACHE_SIZE  = "RAW_REG_CACHE_SIZE"
	BINARY_CACHE_SIZE   = "BINARY_CACHE_SIZE"
	EVTX_FREQUENCY      = "EVTX_FREQUENCY"
	USN_FREQUENCY       = "USN_FREQUENCY"
	ZIP_FILE_CACHE_SIZE = "ZIP_FILE_CACHE_SIZE"
)

type key int

var (
	HuntIdRegex    = regexp.MustCompile(`^H\.[^.]+$`)
	STOP_ITERATION = errors.New("Stop Iteration")
)
