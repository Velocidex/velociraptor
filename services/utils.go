package services

import (
	"context"
	"sync"

	"github.com/Velocidex/ordereddict"
	"www.velocidex.com/golang/velociraptor/artifacts"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	flows_proto "www.velocidex.com/golang/velociraptor/flows/proto"
	"www.velocidex.com/golang/velociraptor/logging"
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
		scope *vfilter.Scope, row *ordereddict.Dict)) error {

	local_wg := &sync.WaitGroup{}
	local_wg.Add(1)

	wg.Add(1)

	go func() {
		defer wg.Done()

		builder := artifacts.ScopeBuilder{
			Config:     config_obj,
			ACLManager: vql_subsystem.NewRoleACLManager("administrator"),
			Env: ordereddict.NewDict().
				Set("artifact_name", artifact_name),
			Logger: logging.NewPlainLogger(config_obj,
				&logging.FrontendComponent),
		}

		scope := builder.Build()
		defer scope.Close()

		// Allow the artifact we are following to be over-ridden by
		// the user.
		custom_artifact_name := "Custom." + artifact_name

		events, cancel := GetJournal().Watch("System.Flow.Completion")
		defer cancel()

		local_wg.Done()

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
					handler(ctx, scope, event)
				}
			}
		}
	}()

	local_wg.Wait()

	return nil
}
