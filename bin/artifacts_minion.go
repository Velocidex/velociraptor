package main

import (
	"fmt"

	"github.com/Velocidex/ordereddict"
	"www.velocidex.com/golang/velociraptor/config"
	logging "www.velocidex.com/golang/velociraptor/logging"
	"www.velocidex.com/golang/velociraptor/services"
	"www.velocidex.com/golang/velociraptor/services/server_artifacts"
	"www.velocidex.com/golang/velociraptor/startup"
	"www.velocidex.com/golang/velociraptor/utils"
)

var (
	artifact_command_minion = artifact_command.Command(
		"minion", "Run a server artifact as a minion")

	artifact_command_minion_org_id = artifact_command_minion.Flag(
		"org_id", "Fetch from the specified org ID "+
			"(default `root`)").
		Default("root").String()

	artifact_command_minion_names = artifact_command_minion.Arg(
		"artifact_name", "The artifact name to collect.").
		Required().HintAction(listArtifactsHint).Strings()

	artifact_command_minion_args = artifact_command_minion.Flag(
		"args", "Artifact args (e.g. --args Foo=Bar).").Strings()
)

func doArtifactCollectMinion() error {
	config_obj, err := makeDefaultConfigLoader().
		WithRequiredFrontend().
		WithRequiredUser().
		WithRequiredLogging().LoadAndValidate()
	if err != nil {
		logging.FlushPrelogs(config.GetDefaultConfig())
		return fmt.Errorf("loading config file: %w", err)
	}

	ctx, cancel := Install_sig_handler()
	defer cancel()

	err = installMinionMode(ctx, config_obj)
	if err != nil {
		return err
	}

	sm, err := startup.StartToolServices(ctx, config_obj)
	if err != nil {
		return err
	}
	defer sm.Close()

	spec, err := parseSpecFromArgs(ctx, config_obj,
		*artifact_command_minion_names,
		*artifact_command_minion_args)
	if err != nil {
		return err
	}

	logger := &LogWriter{config_obj: config_obj}

	// Schedule the server flow.
	results, err := captureQueryWithEnv(ctx, config_obj, `
LET Flow <= collect_client(
   client_id='server',
   artifacts=Artifacts,
   spec=Spec,
   do_not_schedule=TRUE)

SELECT Flow.flow_id AS FlowId, Flow
FROM scope()`,
		logger, ordereddict.NewDict().
			Set("Artifacts", *artifact_command_minion_names).
			Set("Spec", spec))

	if len(results) == 0 {
		return utils.UnknownError
	}

	flow_id, _ := results[0].GetString("FlowId")
	if flow_id == "" {
		return utils.UnknownError
	}

	launcher, err := services.GetLauncher(config_obj)
	if err != nil {
		return err
	}

	collection_context, err := launcher.Storage().LoadCollectionContext(ctx, config_obj,
		"server", flow_id, services.GetFlowOptions{})
	if err != nil {
		return err
	}

	tasks, err := launcher.Storage().GetFlowTasks(
		ctx, config_obj, "server", flow_id, 0, 100)
	if err != nil {
		return err
	}

	worker := &server_artifacts.ServerArtifactWorker{}

	for _, task := range tasks.Items {
		if task.FlowRequest != nil {
			_, err = worker.ProcessUpdateRequest(
				ctx, config_obj,
				&server_artifacts.ServerArtifactRequest{
					Req:               task.FlowRequest,
					CollectionContext: collection_context,
				})
			if err != nil {
				return err
			}
		}
	}

	return nil
}
