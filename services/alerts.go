package services

import (
	"time"

	"github.com/Velocidex/ordereddict"
)

// An alert is a managed object sent by the client's alert() VQL
// function. It is especially handled by the server and routed to a
// central artifact.
type AlertMessage struct {
	// Sent by the client
	ClientId  string            `json:"client_id,omitempty"`
	AlertName string            `json:"name"`
	Timestamp time.Time         `json:"timestamp"`
	EventData *ordereddict.Dict `json:"event_data"`

	// Used to link to the original data source
	Artifact     string `json:"artifact,omitempty"`
	ArtifactType string `json:"artifact_type,omitempty"`
	FlowId       string `json:"flow_id,omitempty"`

	// Managed by the server
	AssignedToUser string `json:"assigned_user,omitempty"`
	Actioned       bool   `json:"actioned,omitempty"`
}
