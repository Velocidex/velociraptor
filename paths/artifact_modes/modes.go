package artifact_modes

import "strings"

type ArtifactMode int

const (
	// The different types of artifacts.
	MODE_INVALID = ArtifactMode(iota)
	MODE_CLIENT
	MODE_CLIENT_EVENT
	MODE_SERVER
	MODE_SERVER_EVENT
	MODE_NOTEBOOK
	MODE_INTERNAL
)

func (self ArtifactMode) String() string {
	switch self {
	case MODE_CLIENT:
		return "CLIENT"
	case MODE_CLIENT_EVENT:
		return "CLIENT_EVENT"
	case MODE_SERVER:
		return "SERVER"
	case MODE_SERVER_EVENT:
		return "SERVER_EVENT"
	case MODE_NOTEBOOK:
		return "NOTEBOOK"
	case MODE_INTERNAL:
		return "INTERNAL"
	default:
		return "INVALID"
	}
}

func IsModeValid(mode ArtifactMode) bool {
	switch mode {
	case MODE_CLIENT, MODE_CLIENT_EVENT, MODE_SERVER,
		MODE_SERVER_EVENT, MODE_NOTEBOOK, MODE_INTERNAL:
		return true
	}
	return false
}

func IsEvent(mode ArtifactMode) bool {
	switch mode {
	// These are regular artifacts
	case MODE_CLIENT, MODE_SERVER, MODE_NOTEBOOK:
		return false

		// These are all event artifacts
	case MODE_SERVER_EVENT, MODE_CLIENT_EVENT, MODE_INTERNAL:
		return true

	default:
		return true
	}
}

func ModeNameToMode(name string) ArtifactMode {
	name = strings.ToUpper(name)
	switch name {
	case "CLIENT":
		return MODE_CLIENT
	case "CLIENT_EVENT":
		return MODE_CLIENT_EVENT
	case "SERVER":
		return MODE_SERVER
	case "SERVER_EVENT":
		return MODE_SERVER_EVENT
	case "NOTEBOOK":
		return MODE_NOTEBOOK
	case "INTERNAL":
		return MODE_INTERNAL
	}
	return MODE_INVALID
}
