package vfs_service

import (
	"context"
	"sync"

	"github.com/Velocidex/ordereddict"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	"www.velocidex.com/golang/velociraptor/constants"
	"www.velocidex.com/golang/velociraptor/flows/proto"
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
	artifact_name, watcher_name string,
	handler func(ctx context.Context,
		config_obj *config_proto.Config,
		scope vfilter.Scope, row *ordereddict.Dict,
		flow *proto.ArtifactCollectorContext)) error {

	journal, err := services.GetJournal(config_obj)
	if err != nil {
		return err
	}

	events, cancel := journal.Watch(
		ctx, "System.Flow.Completion", watcher_name)

	logger := logging.GetLogger(config_obj, &logging.FrontendComponent)

	wg.Add(1)
	go func() {
		defer wg.Done()
		defer cancel()

		defer logger.Info("Stopping watch for %v", artifact_name)

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

		for {
			select {
			case <-ctx.Done():
				return

			case event, ok := <-events:
				if !ok {
					return
				}

				flow := &flows_proto.ArtifactCollectorContext{}
				flow_any, pres := event.Get("Flow")
				if !pres {
					continue
				}

				err := utils.ParseIntoProtobuf(flow_any, flow)
				if err != nil {
					continue
				}

				if shouldForwardFlow(flow, artifact_name) {
					handler(ctx, config_obj, scope, event, flow)
				}
			}
		}
	}()

	return nil
}

func shouldForwardFlow(flow *flows_proto.ArtifactCollectorContext,
	artifact_name string) bool {
	if utils.InString(flow.ArtifactsWithResults, artifact_name) {
		return true
	}

	// Allow the artifact we are following to be over-ridden by
	// the user.
	custom_artifact_name := constants.ARTIFACT_CUSTOM_NAME_PREFIX + artifact_name

	if utils.InString(flow.ArtifactsWithResults, custom_artifact_name) {
		return true
	}
	// Forward empty flows that returned no results as well.
	if flow.Request != nil &&
		utils.InString(flow.Request.Artifacts, artifact_name) {
		return true
	}

	return false
}
