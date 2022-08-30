package services

import (
	"context"

	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
)

// The Label service is responsible for manipulating client's labels
// in a fast and efficient manner.

func GetLabeler(config_obj *config_proto.Config) Labeler {
	org_manager, err := GetOrgManager()
	if err != nil {
		return nil
	}

	l, _ := org_manager.Services(config_obj.OrgId).Labeler()
	return l
}

type Labeler interface {

	// Get the last time any labeling operation modified the
	// client's labels.
	LastLabelTimestamp(
		ctx context.Context,
		config_obj *config_proto.Config,
		client_id string) uint64

	// Is the label set for this client.
	IsLabelSet(
		ctx context.Context,
		config_obj *config_proto.Config,
		client_id string, label string) bool

	// Set the label
	SetClientLabel(
		ctx context.Context,
		config_obj *config_proto.Config,
		client_id, label string) error

	// Remove the label from the client.
	RemoveClientLabel(
		ctx context.Context,
		config_obj *config_proto.Config,
		client_id, label string) error

	// Gets all the labels in a client.
	GetClientLabels(
		ctx context.Context,
		config_obj *config_proto.Config,
		client_id string) []string
}
