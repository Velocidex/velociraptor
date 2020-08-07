package launcher

import (
	"context"

	errors "github.com/pkg/errors"
	actions_proto "www.velocidex.com/golang/velociraptor/actions/proto"
	"www.velocidex.com/golang/velociraptor/artifacts"
	artifacts_proto "www.velocidex.com/golang/velociraptor/artifacts/proto"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	flows_proto "www.velocidex.com/golang/velociraptor/flows/proto"
	"www.velocidex.com/golang/velociraptor/services"
)

type defaultLauncher struct{}

func (self defaultLauncher) SetFlowIdForTests(flow_id string) {}

func (self defaultLauncher) EnsureToolsDeclared(
	ctx context.Context, config_obj *config_proto.Config,
	artifact *artifacts_proto.Artifact) error {
	return errors.New("Launcher not initialized")
}

func (self defaultLauncher) CompileCollectorArgs(
	ctx context.Context,
	config_obj *config_proto.Config,
	principal string,
	repository *artifacts.Repository,
	collector_request *flows_proto.ArtifactCollectorArgs) (
	*actions_proto.VQLCollectorArgs, error) {

	return nil, errors.New("Launcher not initialized")
}

func (self defaultLauncher) ScheduleArtifactCollection(
	ctx context.Context,
	config_obj *config_proto.Config,
	principal string,
	repository *artifacts.Repository,
	collector_request *flows_proto.ArtifactCollectorArgs) (string, error) {
	return "", errors.New("Launcher not initialized")
}

func init() {
	services.RegisterLauncher(defaultLauncher{})
}
