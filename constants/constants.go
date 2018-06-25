package constants

var (
	ENROLLMENT_WELL_KNOWN_FLOW = "aff4:/flows/E:Enrol"

	FLOW_RUNNER_ARGS = "aff4:flow_runner_args"
	FLOW_CONTEXT     = "aff4:flow_context"
	FLOW_STATE       = "aff4:velociraptor_flow_state"
	AFF4_TYPE        = "aff4:type"

	CLIENT_VELOCIRAPTOR_INFO = "aff4:velociraptor_info"

	// The GUI uses this as the client index.
	CLIENT_INDEX_URN = "aff4:/client_index/"

	// GRR Stores certificates but Velociraptor just stores the
	// public key.
	CLIENT_PUBLIC_KEY     = "metadata:public_key"
	CLIENT_LAST_TIMESTAMP = "metadata:ping"

	ATTRS_CLIENT_KEYS = []string{
		"metadata:public_key",
	}

	// The basic information about the client - retrieved by the
	// Interrogate flow.
	ATTR_BASIC_CLIENT_INFO = []string{
		CLIENT_VELOCIRAPTOR_INFO,
	}
)
