package constants

// The following are environment variables which are used to control
// Velociraptor.

const (
	// Set to emulate a slow filesystem. Only used for testing/debugging.
	VELOCIRAPTOR_SLOW_FILESYSTEM = "VELOCIRAPTOR_SLOW_FILESYSTEM"

	// The config file can be read from this environment variable.
	VELOCIRAPTOR_LITERAL_CONFIG = "VELOCIRAPTOR_LITERAL_CONFIG"

	// Config file can be read from these
	VELOCIRAPTOR_CONFIG     = "VELOCIRAPTOR_CONFIG"
	VELOCIRAPTOR_API_CONFIG = "VELOCIRAPTOR_API_CONFIG"

	// Use this to disable CSRF check in the GUI - only for testing.
	VELOCIRAPTOR_DISABLE_CSRF = "VELOCIRAPTOR_DISABLE_CSRF"

	// Inject API latency - for testing.
	VELOCIRAPTOR_INJECT_API_SLEEP = "VELOCIRAPTOR_INJECT_API_SLEEP"
)
