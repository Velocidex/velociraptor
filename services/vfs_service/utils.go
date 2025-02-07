package vfs_service

import (
	"context"
	"fmt"
	"sync"

	"github.com/Velocidex/ordereddict"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	"www.velocidex.com/golang/velociraptor/constants"
	"www.velocidex.com/golang/velociraptor/flows/proto"
	flows_proto "www.velocidex.com/golang/velociraptor/flows/proto"
	"www.velocidex.com/golang/velociraptor/logging"
	"www.velocidex.com/golang/velociraptor/services"
	vjournal "www.velocidex.com/golang/velociraptor/services/journal"
	"www.velocidex.com/golang/velociraptor/utils"
	"www.velocidex.com/golang/velociraptor/vql/acl_managers"
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
		ctx, "System.Flow.Completion",
		fmt.Sprintf("%s for %s", watcher_name, artifact_name))

	logger := logging.GetLogger(config_obj, &logging.FrontendComponent)

	wg.Add(1)
	go func() {
		defer wg.Done()
		defer cancel()

		defer logger.Info("<red>Stopping</> watch for %v for %v (%v)",
			artifact_name, services.GetOrgName(config_obj), watcher_name)

		builder := services.ScopeBuilder{
			Config:     config_obj,
			ACLManager: acl_managers.NewRoleACLManager(config_obj, "administrator"),
			Env: ordereddict.NewDict().
				Set("artifact_name", artifact_name),
			Logger: logging.NewPlainLogger(config_obj,
				&logging.FrontendComponent),
		}

		manager, err := services.GetRepositoryManager(config_obj)
		if err != nil {
			logger.Error("watchForFlowCompletion: %v", err)
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

				// Extract the flow description from the event.
				flow, err := vjournal.GetFlowFromQueue(ctx, config_obj, event)
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
