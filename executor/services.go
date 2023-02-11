package executor

/*
func StartEventTableService(
	ctx context.Context,
	wg *sync.WaitGroup,
	config_obj *config_proto.Config,
	exe *ClientExecutor,
	output_chan chan *crypto_proto.VeloMessage) error {

	logger := logging.GetLogger(config_obj, &logging.ClientComponent)
	logger.Info("<green>Starting</> event query service with version %v.",
		actions.GlobalEventTableVersion())

	actions.InitializeEventTable(ctx, config_obj, output_chan, wg)

	writeback, _ := config.GetWriteback(config_obj.Client)
	if writeback != nil && writeback.EventQueries != nil {
		actions.UpdateEventTable{}.Run(config_obj, ctx,
			exe.MonitoringManager(), output_chan, writeback.EventQueries)
	}

	logger.Info("<green>Starting</> event query service with version %v.",
		actions.GlobalEventTableVersion())

	return nil
}
*/
