/*
Velociraptor - Dig Deeper
Copyright (C) 2019-2025 Rapid7 Inc.

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
	VERSION = "0.76.1-rc1"

	// This is the version of dependent client binaries that will be
	// included in the offline collector or MSI. Usually this will be
	// lockstep with the server version except for server side
	// patches.
	CLIENT_VERSION = VERSION

	ENROLLMENT_WELL_KNOWN_FLOW   = "E:Enrol"
	MONITORING_WELL_KNOWN_FLOW   = FLOW_PREFIX + "Monitoring"
	STATUS_CHECK_WELL_KNOWN_FLOW = FLOW_PREFIX + "Status"

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
	MAX_MEMORY    = 50 * 1024 * 1024
	MAX_POST_SIZE = 5 * 1024 * 1024

	// Messages to the client which we dont care about their responses.
	IgnoreResponseState = uint64(101)

	USER_AGENT = "Velociraptor"

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
	SCOPE_REPOSITORY        = "$repository"
	SCOPE_RESPONDER_CONTEXT = "_Context"
	SCOPE_QUERY_NAME        = "$query_name"

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
	NTFS_CACHE_SIZE                   = "NTFS_CACHE_SIZE"
	NTFS_MAX_DIRECTORY_DEPTH          = "NTFS_MAX_DIRECTORY_DEPTH"
	NTFS_MAX_LINKS                    = "NTFS_MAX_LINKS"
	NTFS_INCLUDE_SHORT_NAMES          = "NTFS_INCLUDE_SHORT_NAMES"
	NTFS_DISABLE_FULL_PATH_RESOLUTION = "NTFS_DISABLE_FULL_PATH_RESOLUTION"

	// VSS Analysis
	// Max age of VSS in (int) days we will consider.
	VSS_MAX_AGE_DAYS = "VSS_MAX_AGE_DAYS"

	// Controls the lifetime of the registry cache.
	REG_CACHE_SIZE = "REG_CACHE_SIZE"
	REG_CACHE_TIME = "REG_CACHE_TIME"

	// Maximum size of files to hash
	HASH_MAX_SIZE   = "HASH_MAX_SIZE"
	BUFFER_MAX_SIZE = "BUFFER_MAX_SIZE"

	RAW_REG_CACHE_SIZE  = "RAW_REG_CACHE_SIZE"
	RAW_REG_CACHE_TIME  = "RAW_REG_CACHE_TIME"
	BINARY_CACHE_SIZE   = "BINARY_CACHE_SIZE"
	EVTX_FREQUENCY      = "EVTX_FREQUENCY"
	USN_FREQUENCY       = "USN_FREQUENCY"
	ZIP_FILE_CACHE_SIZE = "ZIP_FILE_CACHE_SIZE"

	PST_CACHE_SIZE = "PST_CACHE_SIZE"
	PST_CACHE_TIME = "PST_CACHE_TIME"

	// Path to disk based process tracker cache
	PROCESS_TRACKER_CACHE = "PROCESS_TRACKER_CACHE"

	// Sets client uploaders to be async and resumable.
	UPLOAD_IS_RESUMABLE = "UPLOAD_IS_RESUMABLE"

	// The result names for the upload resumption
	UPLOAD_RESUMED_SOURCE = "Server.Internal.ResumedUploads"

	// Used by the SSH accessor to configure access
	SSH_CONFIG = "SSH_CONFIG"

	// Used by the SMB accessor to configure credentials.
	SMB_CREDENTIALS = "SMB_CREDENTIALS"

	// Used by the S3 accessor to configure credentials.
	S3_CREDENTIALS = "S3_CREDENTIALS"

	// Used by the overlay accessor to configure delegates'
	OVERLAY_ACCESSOR_DELEGATES = "OVERLAY_ACCESSOR_DELEGATES"

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
	VQL_ERROR_REGEX = "(?i)(Error:|Symbol.+?not found|Expecting a path arg type, not|Field.+?is required|Unexpected arg)"

	// Set in the scope with one or more passwords. Used by the zip
	// accessor to open password protected zip files.
	ZIP_PASSWORDS = "ZIP_PASSWORDS"

	// If this is set, the logs will report the decrypted password
	REPORT_ZIP_PASSWORD = "REPORT_ZIP_PASSWORD"

	// If this is set we always copy SQLite files to a tempfile. Used
	// by the sqlite() plugin.
	SQLITE_ALWAYS_MAKE_TEMPFILE = "SQLITE_ALWAYS_MAKE_TEMPFILE"

	// This variable in the scope can set a dict that maps columns to
	// their types. For example
	// LET ColumnTypes <= dict(Column1="preview_upload")
	COLUMN_TYPES = "ColumnTypes"

	// Setting this in the scope causes times to be serialized in the
	// specified timezone instead of UTC. NOTE: The GUI changes times
	// to the timezone specified in the user preferences so this may
	// not be immediately visible in the GUI but will affect the
	// timezone actually serialized.
	TZ = "TZ"

	// Log levels for the Yara plugin:
	// 1: Log ranges
	// 2: Log Bytes scanned with 30 second deduplicated logs
	YARA_LOG_LEVEL = "YARA_LOG_LEVEL"

	// Set this to see extended debug messages of various LRU
	LRU_DEBUG = "LRU_DEBUG"

	PinnedServerName = "VelociraptorServer"

	// Default gateway identity. This is only used when creating the
	// gateway certificates.
	PinnedGwName = "GRPC_GW"

	CLIENT_API_VERSION = uint32(4)

	// The newer client communications from version 0.6.8:
	// * Flow state is maintained on the client.
	// * Flow state is synced to the server periodically.
	// * Moves processing requirements from server to the client -
	//   reducing server load.
	CLIENT_API_VERSION_0_6_8 = uint32(4)

	DISABLE_DANGEROUS_API_CALLS = "DISABLE_DANGEROUS_API_CALLS"

	// Fixed secret types - definitions in the sanity service
	AWS_S3_CREDS    = "AWS S3 Creds"
	SSH_PRIVATE_KEY = "SSH PrivateKey"
	HTTP_SECRETS    = "HTTP Secrets"
	SPLUNK_CREDS    = "Splunk Creds"
	ELASTIC_CREDS   = "Elastic Creds"
	SMTP_CREDS      = "SMTP Creds"
	EXECVE_SECRET   = "Execve Secrets"

	// The name of the annotation timeline
	TIMELINE_ANNOTATION      = "Annotation"
	TIMELINE_DEFAULT_KEY     = "Timestamp"
	TIMELINE_DEFAULT_MESSAGE = "Message"

	// Environment variables
	VELOCIRAPTOR_CONFIG         = "VELOCIRAPTOR_CONFIG"
	VELOCIRAPTOR_LITERAL_CONFIG = "VELOCIRAPTOR_LITERAL_CONFIG"
	VELOCIRAPTOR_API_CONFIG     = "VELOCIRAPTOR_API_CONFIG"
)

type key int

var (
	HuntIdRegex    = regexp.MustCompile(`^H\.[^.]+$`)
	ClientIdRegex  = regexp.MustCompile(`^C\.[^\./ ]+$`)
	STOP_ITERATION = errors.New("Stop Iteration")
)
