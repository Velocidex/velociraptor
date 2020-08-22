package launcher

import (
	"context"

	errors "github.com/pkg/errors"
	actions_proto "www.velocidex.com/golang/velociraptor/actions/proto"
	artifacts_proto "www.velocidex.com/golang/velociraptor/artifacts/proto"
	flows_proto "www.velocidex.com/golang/velociraptor/flows/proto"
	"www.velocidex.com/golang/velociraptor/services"
	vql_subsystem "www.velocidex.com/golang/velociraptor/vql"
)

type defaultLauncher struct{}

func (self defaultLauncher) SetFlowIdForTests(flow_id string) {}

func (self defaultLauncher) EnsureToolsDeclared(
	ctx context.Context, artifact *artifacts_proto.Artifact) error {
	return errors.New("Launcher not initialized")
}

func (self defaultLauncher) CompileCollectorArgs(
	ctx context.Context,
	acl_manager vql_subsystem.ACLManager,
	repository services.Repository,
	collector_request *flows_proto.ArtifactCollectorArgs) (
	*actions_proto.VQLCollectorArgs, error) {
	return nil, errors.New("Launcher not initialized")
}

func (self defaultLauncher) ScheduleArtifactCollection(
	ctx context.Context,
	acl_manager vql_subsystem.ACLManager,
	repository services.Repository,
	collector_request *flows_proto.ArtifactCollectorArgs) (string, error) {
	return "", errors.New("Launcher not initialized")
}

func init() {
	services.RegisterLauncher(defaultLauncher{})
}
