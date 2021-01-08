package vfs_service

import (
	"context"
	"sync"

	"github.com/Velocidex/ordereddict"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	"www.velocidex.com/golang/velociraptor/constants"
	flows_proto "www.velocidex.com/golang/velociraptor/flows/proto"
	"www.velocidex.com/golang/velociraptor/logging"
	"www.velocidex.com/golang/velociraptor/services"
	"www.velocidex.com/golang/velociraptor/utils"
	vql_subsystem "www.velocidex.com/golang/velociraptor/vql"
	"www.velocidex.com/golang/vfilter"
)

// Watch the System.Flow.Completion queue for specific artifacts and
// run the handlers on the rows.
func watchForFlowCompletion(
	ctx context.Context,
	wg *sync.WaitGroup,
	config_obj *config_proto.Config,
	artifact_name string,
	handler func(ctx context.Context,
		config_obj *config_proto.Config,
		scope vfilter.Scope, row *ordereddict.Dict)) error {

	journal, err := services.GetJournal()
	if err != nil {
		return err
	}

	events, cancel := journal.Watch("System.Flow.Completion")

	wg.Add(1)
	go func() {
		defer wg.Done()
		defer cancel()

		builder := services.ScopeBuilder{
			Config:     config_obj,
			ACLManager: vql_subsystem.NewRoleACLManager("administrator"),
			Env: ordereddict.NewDict().
				Set("artifact_name", artifact_name),
			Logger: logging.NewPlainLogger(config_obj,
				&logging.FrontendComponent),
		}

		manager, err := services.GetRepositoryManager()
		if err != nil {
			return
		}

		scope := manager.BuildScope(builder)
		defer scope.Close()

		// Allow the artifact we are following to be over-ridden by
		// the user.
		custom_artifact_name := constants.ARTIFACT_CUSTOM_NAME_PREFIX + artifact_name

		for {
			select {
			case <-ctx.Done():
				return

			case event, ok := <-events:
				if !ok {
					return
				}

				flow := &flows_proto.ArtifactCollectorContext{}
				flow_any, _ := event.Get("Flow")
				err := utils.ParseIntoProtobuf(flow_any, flow)
				if err != nil {
					continue
				}

				if utils.InString(flow.ArtifactsWithResults, artifact_name) ||
					utils.InString(flow.ArtifactsWithResults, custom_artifact_name) {
					handler(ctx, config_obj, scope, event)
				}
			}
		}
	}()

	return nil
}
