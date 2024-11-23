package launcher

import (
	"context"
	"fmt"

	actions_proto "www.velocidex.com/golang/velociraptor/actions/proto"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	flows_proto "www.velocidex.com/golang/velociraptor/flows/proto"
	"www.velocidex.com/golang/velociraptor/services"
	"www.velocidex.com/golang/velociraptor/vql/acl_managers"
)

func (self *Launcher) calculateTraceQuery(
	ctx context.Context, config_obj *config_proto.Config,
	freq uint64) ([]*actions_proto.VQLCollectorArgs, error) {

	// NOTE: Use the built in global Generic.Client.Trace artifact so
	// the server does not need to send it.
	manager, err := services.GetRepositoryManager(config_obj)
	if err != nil {
		return nil, err
	}

	repository, err := manager.GetGlobalRepository(config_obj)
	if err != nil {
		return nil, err
	}

	return self.CompileCollectorArgs(ctx, config_obj,
		acl_managers.NullACLManager{},
		repository, services.CompilerOptions{},
		&flows_proto.ArtifactCollectorArgs{
			AllowCustomOverrides: true,
			Artifacts:            []string{"Generic.Client.Trace"},
			Specs: []*flows_proto.ArtifactSpec{
				{
					Artifact: "Generic.Client.Trace",
					Parameters: &flows_proto.ArtifactParameters{
						Env: []*actions_proto.VQLEnv{{
							Key:   "FrequencySec",
							Value: fmt.Sprintf("%v", freq),
						}},
					},
				}},
		})
}
