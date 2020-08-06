package services

import "sync"

// The Label service is responsible for manipulating client's labels
// in a fast and efficient manner.

var (
	labeler_mu sync.Mutex
	labeler    Labeler
)

func GetLabeler() Labeler {
	labeler_mu.Lock()
	defer labeler_mu.Unlock()

	return labeler
}

func RegisterLabeler(l Labeler) {
	labeler_mu.Lock()
	defer labeler_mu.Unlock()

	labeler = l
}

type Labeler interface {

	// Get the last time any labeling operation modified the
	// client's labels.
	LastLabelTimestamp(client_id string) uint64

	// Is the label set for this client.
	IsLabelSet(client_id string, label string) bool

	// Set the label
	SetClientLabel(client_id, label string) error

	// Remove the label from the client.
	RemoveClientLabel(client_id, label string) error

	// Gets all the labels in a client.
	GetClientLabels(client_id string) []string
}
