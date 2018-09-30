package constants

var (
	VERSION                    = "0.2.4"
	ENROLLMENT_WELL_KNOWN_FLOW = "aff4:/flows/E:Enrol"

	// Temporary attribute
	AFF4_ATTR = "aff4:data"

	FLOW_PREFIX             = "F."
	FOREMAN_WELL_KNOWN_FLOW = "aff4:/flows/E.Foreman"
	HUNTS_URN               = "aff4:/hunts"
	HUNT_PREFIX             = "H."

	// The GUI uses this as the client index.
	CLIENT_INDEX_URN = "aff4:/client_index/"

	USER_URN = "aff4:/users/"

	// Well known flows - Request ID:
	LOG_SINK uint64 = 980
)

const TransferWellKnownFlowId = 5
