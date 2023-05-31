/*
   Velociraptor - Dig Deeper
   Copyright (C) 2019-2022 Rapid7 Inc.

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

var (
	VERSION = "0.7.0-dev"
)

const (
	ENROLLMENT_WELL_KNOWN_FLOW = "E:Enrol"
	MONITORING_WELL_KNOWN_FLOW = FLOW_PREFIX + "Monitoring"

	FLOW_PREFIX             = "F."
	FOREMAN_WELL_KNOWN_FLOW = "E.Foreman"
	HUNT_PREFIX             = "H."
	ORG_PREFIX              = "O"

	// Well known flows - Request ID:
	LOG_SINK   uint64 = 980
	STATS_SINK uint64 = 981

	TransferWellKnownFlowId = 5
	ProcessVQLResponses     = 1

	// Largest buffer we use for comms.
	MAX_MEMORY    = 5 * 1024 * 1024
	MAX_POST_SIZE = 5 * 1024 * 1024

	// Messages to the client which we dont care about their responses.
	IgnoreResponseState = uint64(101)

	USER_AGENT = "Velociraptor - Dig Deeper!"

	// Globals set in VQL scopes.
	SCOPE_CONFIG            = "config"
	SCOPE_SERVER_CONFIG     = "server_config"
	SCOPE_THROTTLE          = "$throttle"
	SCOPE_UPLOADER          = "$uploader"
	SCOPE_RESPONDER         = "$responder"
	SCOPE_MOCK              = "$mock"
	SCOPE_ROOT              = "$root"
	SCOPE_STACK             = "$stack"
	SCOPE_DEVICE_MANAGER    = "$device_manager"
	SCOPE_RESPONDER_CONTEXT = "_Context"

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
	NTFS_CACHE_SIZE          = "NTFS_CACHE_SIZE"
	NTFS_MAX_DIRECTORY_DEPTH = "NTFS_MAX_DIRECTORY_DEPTH"
	NTFS_MAX_LINKS           = "NTFS_MAX_LINKS"
	NTFS_INCLUDE_SHORT_NAMES = "NTFS_INCLUDE_SHORT_NAMES"

	RAW_REG_CACHE_SIZE  = "RAW_REG_CACHE_SIZE"
	BINARY_CACHE_SIZE   = "BINARY_CACHE_SIZE"
	EVTX_FREQUENCY      = "EVTX_FREQUENCY"
	USN_FREQUENCY       = "USN_FREQUENCY"
	ZIP_FILE_CACHE_SIZE = "ZIP_FILE_CACHE_SIZE"

	// VQL tries to balance memory/cpu tradeoffs and also place limits
	// on memory use. These parameters control this behavior. You can
	// set them in the VQL environment to influence how the engine
	// optimizes the queries.

	// Holds this many rows in memory (default 1000) before switching
	// to disk backing to limit memory use.
	VQL_MATERIALIZE_ROW_LIMIT = "VQL_MATERIALIZE_ROW_LIMIT"

	// Certain VQL errors represent a failure in artifact
	// collection. We use this RegExp to determine if log messages
	// represent failure.
	VQL_ERROR_REGEX = "(?i)(Error:|Symbol.+?not found|Expecting a path arg type, not)"

	// Set in the scope with one or more passwords
	ZIP_PASSWORDS = "ZIP_PASSWORDS"

	// If this is set we always copy SQLite files to a tempfile
	SQLITE_ALWAYS_MAKE_TEMPFILE = "SQLITE_ALWAYS_MAKE_TEMPFILE"

	PinnedServerName = "VelociraptorServer"

	CLIENT_API_VERSION = uint32(4)

	// The newer client communications from version 0.6.8:
	// * Flow state is maintained on the client.
	// * Flow state is synced to the server periodically.
	// * Moves processing requirements from server to the client -
	//   reducing server load.
	CLIENT_API_VERSION_0_6_8 = uint32(4)

	DISABLE_DANGEROUS_API_CALLS = "DISABLE_DANGEROUS_API_CALLS"
)

type key int

var (
	HuntIdRegex    = regexp.MustCompile(`^H\.[^.]+$`)
	ClientIdRegex  = regexp.MustCompile(`^C\.[^\./ ]+$`)
	STOP_ITERATION = errors.New("Stop Iteration")
)
