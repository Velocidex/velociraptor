package artifact_modes

import (
	"fmt"
)

// A QueueName represents an event queue which can be communicated
// on. The QueueName is derived from an artifact and its type.
type QueueName string

func NewQueueName(artifact string, mode ArtifactMode) QueueName {
	return QueueName(fmt.Sprintf("%s:%s", artifact, mode.String()))
}
