package services

import (
	"context"

	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	crypto_proto "www.velocidex.com/golang/velociraptor/crypto/proto"
	flows_proto "www.velocidex.com/golang/velociraptor/flows/proto"
)

func GetServerArtifactRunner(config_obj *config_proto.Config) (ServerArtifactRunner, error) {
	org_manager, err := GetOrgManager()
	if err != nil {
		return nil, err
	}

	return org_manager.Services(config_obj.OrgId).ServerArtifactRunner()
}

type ServerArtifactRunner interface {
	// Start a new collection in the current process.
	LaunchServerArtifact(
		config_obj *config_proto.Config,
		session_id string,
		req *crypto_proto.FlowRequest,
		collection_context *flows_proto.ArtifactCollectorContext) error

	// Cancel the current running collection if possible.
	Cancel(ctx context.Context, session_id, principal string)
}
