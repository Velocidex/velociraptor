package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	errors "github.com/pkg/errors"
	kingpin "gopkg.in/alecthomas/kingpin.v2"
	actions_proto "www.velocidex.com/golang/velociraptor/actions/proto"
	api_proto "www.velocidex.com/golang/velociraptor/api/proto"
	artifacts "www.velocidex.com/golang/velociraptor/artifacts"
	"www.velocidex.com/golang/velociraptor/file_store/csv"
	logging "www.velocidex.com/golang/velociraptor/logging"
	vql_networking "www.velocidex.com/golang/velociraptor/vql/networking"
	vfilter "www.velocidex.com/golang/vfilter"
)

var (
	artifact_command_acquire = artifact_command.Command(
		"acquire", "Acquire artifacts into files.")

	artifact_command_acquire_dump_dir = artifact_command_acquire.Flag(
		"outdir", "Directory to dump output files.").
		Default(".").String()

	artifact_command_acquire_names = artifact_command_acquire.Arg(
		"names", "A list of artifacts to collect.").
		Required().Strings()

	artifact_command_acquire_parameters = artifact_command_acquire.Flag(
		"parameters", "Parameters to set for the artifacts.").
		Short('p').StringMap()
)

func acquireArtifact(ctx context.Context, config_obj *api_proto.Config,
	name string, request *actions_proto.VQLCollectorArgs) error {
	logger := logging.NewLogger(config_obj)
	subdir := filepath.Join(*artifact_command_acquire_dump_dir, name)

	err := os.MkdirAll(subdir, 0700)
	if err != nil {
		return errors.Wrap(err, "Create output directory")
	}

	logger.Info("Collecting artifact %v into subdir %v\n", name, subdir)

	env := vfilter.NewDict().
		Set("config", config_obj.Client).
		Set("server_config", config_obj).
		Set("$uploader", &vql_networking.FileBasedUploader{
			UploadDir: filepath.Join(subdir, "files"),
		})

	// Allow the user to override the env - this is how we set
	// artifact parameters.
	for _, request_env := range request.Env {
		env.Set(request_env.Key, request_env.Value)
	}

	for k, v := range *artifact_command_acquire_parameters {
		env.Set(k, v)
	}

	repository := getRepository(config_obj)
	scope := artifacts.MakeScope(repository).AppendVars(env)
	scope.Logger = logging.NewPlainLogger(config_obj)

	now := time.Now()
	fd, err := os.OpenFile(
		filepath.Join(subdir,
			fmt.Sprintf("%d-%02d-%02d.csv", now.Year(),
				now.Month(), now.Day())),
		os.O_RDWR|os.O_CREATE, 0600)
	if err != nil {
		return err
	}
	defer fd.Close()

	writer, err := csv.GetCSVWriter(scope, fd)
	defer writer.Close()

	for _, query := range request.Query {
		vql, err := vfilter.Parse(query.VQL)
		if err != nil {
			return err
		}

		row_chan := vql.Eval(ctx, scope)
	run_query:
		for {
			select {
			case <-ctx.Done():
				return nil

			case row, ok := <-row_chan:
				if !ok {
					break run_query
				}
				writer.Write(row)
			}
		}
	}

	return nil
}

func doArtifactsAcquire() {
	config_obj := get_config_or_default()
	repository := getRepository(config_obj)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	logger := logging.NewLogger(config_obj)

	var wg sync.WaitGroup

	for _, name := range *artifact_command_acquire_names {
		artifact, pres := repository.Get(name)
		if !pres {
			kingpin.Fatalf("Artifact %v not found", name)
		}

		request := &actions_proto.VQLCollectorArgs{}
		err := artifacts.Compile(artifact, request)
		kingpin.FatalIfError(
			err, fmt.Sprintf("Unable to compile artifact %s.", name))

		wg.Add(1)
		go func(name string, request *actions_proto.VQLCollectorArgs) {
			defer wg.Done()
			err := acquireArtifact(ctx, config_obj, name, request)
			if err != nil {
				logger.Error(fmt.Sprintf(
					"While collecting artifact %v", name), err)
			}
		}(name, request)
	}

	// Wait for all collections to complete.
	wg.Wait()
}

func init() {
	command_handlers = append(command_handlers, func(command string) bool {
		switch command {
		case artifact_command_acquire.FullCommand():
			doArtifactsAcquire()

		default:
			return false
		}
		return true
	})
}
