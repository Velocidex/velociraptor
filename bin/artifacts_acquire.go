/*
   Velociraptor - Hunting Evil
   Copyright (C) 2019 Velocidex Innovations.

   This program is free software: you can redistribute it and/or modify
   it under the terms of the GNU Affero General Public License as published
   by the Free Software Foundation, either version 3 of the License, or
   (at your option) any later version.

   This program is distributed in the hope that it will be useful,
   but WITHOUT ANY WARRANTY; without even the implied warranty of
   MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
   GNU Affero General Public License for more details.

   You should have received a copy of the GNU Affero General Public License
   along with this program.  If not, see <https://www.gnu.org/licenses/>.
*/
package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/Velocidex/ordereddict"
	errors "github.com/pkg/errors"
	kingpin "gopkg.in/alecthomas/kingpin.v2"
	actions_proto "www.velocidex.com/golang/velociraptor/actions/proto"
	artifacts "www.velocidex.com/golang/velociraptor/artifacts"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	"www.velocidex.com/golang/velociraptor/file_store/csv"
	logging "www.velocidex.com/golang/velociraptor/logging"
	"www.velocidex.com/golang/velociraptor/uploads"
	vql_subsystem "www.velocidex.com/golang/velociraptor/vql"
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

func acquireArtifact(ctx context.Context, config_obj *config_proto.Config,
	name string, request *actions_proto.VQLCollectorArgs) error {
	logger := logging.GetLogger(config_obj, &logging.ToolComponent)
	subdir := filepath.Join(*artifact_command_acquire_dump_dir, name)

	err := os.MkdirAll(subdir, 0700)
	if err != nil {
		return errors.Wrap(err, "Create output directory")
	}

	logger.Info("Collecting artifact %v into subdir %v", name, subdir)

	builder := artifacts.ScopeBuilder{
		Config:     config_obj,
		ACLManager: vql_subsystem.NullACLManager{},
		Logger:     log.New(&LogWriter{config_obj}, "Velociraptor: ", log.Lshortfile),
		Env:        ordereddict.NewDict(),
		Uploader: &uploads.FileBasedUploader{
			UploadDir: filepath.Join(subdir, "files"),
		},
	}

	if *run_as != "" {
		builder.ACLManager = vql_subsystem.NewServerACLManager(config_obj, *run_as)
	}

	// Allow the user to override the env - this is how we set
	// artifact parameters.
	for _, request_env := range request.Env {
		builder.Env.Set(request_env.Key, request_env.Value)
	}

	for k, v := range *artifact_command_acquire_parameters {
		builder.Env.Set(k, v)
	}

	scope := builder.Build()
	defer scope.Close()

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

	writer := csv.GetCSVAppender(scope, fd, true /* write_headers */)
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
	config_obj := load_config_or_default()
	repository, err := getRepository(config_obj)
	kingpin.FatalIfError(err, "Loading extra artifacts")

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	logger := logging.GetLogger(config_obj, &logging.ToolComponent)

	var wg sync.WaitGroup

	for _, name := range *artifact_command_acquire_names {
		artifact, pres := repository.Get(name)
		if !pres {
			kingpin.Fatalf("Artifact %v not found", name)
		}

		request := &actions_proto.VQLCollectorArgs{MaxWait: uint64(*max_wait)}
		err := repository.Compile(artifact, request)
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
