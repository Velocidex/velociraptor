package services

import (
	"context"
	"sync"

	"github.com/Velocidex/ordereddict"
	"www.velocidex.com/golang/velociraptor/artifacts"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	"www.velocidex.com/golang/velociraptor/logging"
	"www.velocidex.com/golang/velociraptor/utils"
	vql_subsystem "www.velocidex.com/golang/velociraptor/vql"
	"www.velocidex.com/golang/vfilter"
)

// Watch the System.Flow.Completion queue for a specific artifacts and
// run the handlers on the rows.
func watchForFlowCompletion(
	ctx context.Context,
	wg *sync.WaitGroup,
	config_obj *config_proto.Config,
	artifact_name string,
	handler func(ctx context.Context,
		scope *vfilter.Scope, row vfilter.Row)) error {

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

	vql, err := vfilter.Parse("select * FROM " +
		"watch_monitoring(artifact='System.Flow.Completion') " +
		"WHERE Flow.artifacts_with_results =~ artifact_name")
	if err != nil {
		return err
	}

	wg.Add(1)
	go func() {
		defer wg.Done()

		defer utils.CheckForPanic("watchForFlowCompletion: %v", artifact_name)

		for row := range vql.Eval(ctx, scope) {
			handler(ctx, scope, row)
		}
	}()

	return nil
}
