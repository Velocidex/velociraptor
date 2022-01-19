package main

import (
	"context"
	"fmt"
	"strconv"
	"sync"
	"time"

	actions_proto "www.velocidex.com/golang/velociraptor/actions/proto"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	crypto_proto "www.velocidex.com/golang/velociraptor/crypto/proto"
	"www.velocidex.com/golang/velociraptor/datastore"
	"www.velocidex.com/golang/velociraptor/file_store/api"
	logging "www.velocidex.com/golang/velociraptor/logging"
	"www.velocidex.com/golang/velociraptor/paths"
	"www.velocidex.com/golang/velociraptor/search"
	"www.velocidex.com/golang/velociraptor/server"
	"www.velocidex.com/golang/velociraptor/services"
	"www.velocidex.com/golang/velociraptor/services/client_info"
	"www.velocidex.com/golang/velociraptor/services/frontend"
	"www.velocidex.com/golang/velociraptor/services/indexing"
	"www.velocidex.com/golang/velociraptor/services/journal"
	"www.velocidex.com/golang/velociraptor/services/labels"
	vql_subsystem "www.velocidex.com/golang/velociraptor/vql"
)

var (
	vacuum_command = app.Command(
		"vacuum", "Clean up the datastore and remove old items.")

	vacuum_command_generate = vacuum_command.
				Flag("generate", "Generate this many test tasks").Int()

	vacuum_command_age = vacuum_command.
				Flag("age", "Remove tasks older than this many seconds").
				Default("1000000").Int()
)

func doVacuum() error {
	config_obj, err := makeDefaultConfigLoader().
		WithRequiredFrontend().
		WithRequiredUser().
		WithRequiredLogging().LoadAndValidate()
	if err != nil {
		return fmt.Errorf("loading config file: %w", err)
	}

	ctx, cancel := install_sig_handler()
	defer cancel()

	sm := services.NewServiceManager(ctx, config_obj)
	defer sm.Close()

	logger := logging.GetLogger(config_obj, &logging.FrontendComponent)
	logger.Info("<green>Will start vacuuming datastore</>")

	// Increase resource limits.
	server.IncreaseLimits(config_obj)

	err = sm.Start(datastore.StartMemcacheFileService)
	if err != nil {
		return err
	}

	err = sm.Start(journal.StartJournalService)
	if err != nil {
		return err
	}

	err = sm.Start(frontend.StartFrontendService)
	if err != nil {
		return err
	}

	err = sm.Start(labels.StartLabelService)
	if err != nil {
		return err
	}

	err = sm.Start(client_info.StartClientInfoService)
	if err != nil {
		return err
	}

	err = sm.Start(indexing.StartIndexingService)
	if err != nil {
		return err
	}

	if *vacuum_command_generate > 0 {
		return generateTasks(ctx, config_obj, *vacuum_command_generate)
	}

	return deleteTasks(ctx, config_obj)
}

func generateTasks(
	ctx context.Context, config_obj *config_proto.Config,
	number int) error {
	client_info_manager, err := services.GetClientInfoManager()
	if err != nil {
		return err
	}
	_ = client_info_manager

	scope := vql_subsystem.MakeScope()

	// Get all the clients from the index.
	client_chan, err := search.SearchClientsChan(ctx, scope, config_obj, "C.", "")
	if err != nil {
		return err
	}

	for client_info := range client_chan {
		tasks := make([]*crypto_proto.VeloMessage, 0, number)
		for i := 0; i < number; i++ {
			tasks = append(tasks, &crypto_proto.VeloMessage{
				UpdateForeman: &actions_proto.ForemanCheckin{
					LastHuntTimestamp: 12212,
				},
			})
		}
		fmt.Printf("ClientInfo %v %v\n", client_info.ClientId,
			client_info.OsInfo.Hostname)
		err = client_info_manager.QueueMessagesForClient(
			client_info.ClientId, tasks, false)
		if err != nil {
			fmt.Printf("Error: %v\n", err)
		}
	}

	return nil
}

func deleteTasks(
	ctx context.Context, config_obj *config_proto.Config) error {
	// We want to get all tasks
	config_obj.Datastore.MaxDirSize = 100000000

	db, err := datastore.GetDB(config_obj)
	if err != nil {
		return err
	}

	scope := vql_subsystem.MakeScope()

	sub_ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	// Get all the clients from the index.
	client_chan, err := search.SearchClientsChan(
		sub_ctx, scope, config_obj, "all", "")
	if err != nil {
		return err
	}

	// Create a worker pool to handle the tasks.
	wg := &sync.WaitGroup{}
	tasks_chan := make(chan api.DSPathSpec)
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go processTask(tasks_chan, wg, config_obj)
	}

	count := 0
	for client_info := range client_chan {
		client_path_manager := paths.NewClientPathManager(client_info.ClientId)
		tasks, err := db.ListChildren(
			config_obj, client_path_manager.TasksDirectory())
		if err != nil {
			return err
		}

		fmt.Printf("Client %v has %v tasks...\n", client_info.ClientId, len(tasks))
		for _, t := range tasks {
			select {
			case <-sub_ctx.Done():
				break

			case tasks_chan <- t:
			}
		}
		count++
	}

	close(tasks_chan)
	wg.Wait()

	fmt.Printf("Processed %v clients\n", count)

	return nil
}

func processTask(task_chan <-chan api.DSPathSpec, wg *sync.WaitGroup,
	config_obj *config_proto.Config) {
	defer wg.Done()

	db, err := datastore.GetDB(config_obj)
	if err != nil {
		return
	}

	// Remove old tasks,
	now := time.Now()

	for task := range task_chan {
		// The task id is a nano timestamp
		id, err := strconv.ParseInt(task.Base(), 0, 64)
		if err != nil {
			continue
		}

		// If the task is too old we dont even need to load it from
		// storage - just delete it already.
		timestamp := time.Unix(0, id)
		if timestamp.Add(
			time.Duration(*vacuum_command_age) * time.Second).Before(now) {

			wg.Add(1)
			fmt.Printf("Deleting old task %v timestamp %v\n",
				task.AsClientPath(), timestamp)
			db.DeleteSubjectWithCompletion(config_obj, task, wg.Done)
			continue
		}

		// Load the task and check if it is an
		task_obj := &crypto_proto.VeloMessage{}
		err = db.GetSubject(config_obj, task, task_obj)
		if err == nil &&
			task_obj.UpdateForeman == nil &&
			task_obj.UpdateEventTable == nil {
			continue
		}

		// Here we have unreadable files, or Foreman or Event table
		// updates. We dont really need to keep those because they
		// will be reissued when the client connects next time.
		wg.Add(1)
		db.DeleteSubjectWithCompletion(config_obj, task, wg.Done)
	}
}

func init() {
	command_handlers = append(command_handlers, func(command string) bool {
		switch command {
		case vacuum_command.FullCommand():
			FatalIfError(vacuum_command, doVacuum)

		default:
			return false
		}

		return true
	})
}
