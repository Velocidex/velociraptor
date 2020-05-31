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
	"io/ioutil"
	"log"
	"os"
	"path/filepath"
	"strings"

	"github.com/Velocidex/ordereddict"
	"github.com/Velocidex/yaml/v2"
	"github.com/sergi/go-diff/diffmatchpatch"
	kingpin "gopkg.in/alecthomas/kingpin.v2"
	actions_proto "www.velocidex.com/golang/velociraptor/actions/proto"
	artifacts "www.velocidex.com/golang/velociraptor/artifacts"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	"www.velocidex.com/golang/velociraptor/constants"
	flows_proto "www.velocidex.com/golang/velociraptor/flows/proto"
	"www.velocidex.com/golang/velociraptor/reporting"
	"www.velocidex.com/golang/velociraptor/services"
	vql_subsystem "www.velocidex.com/golang/velociraptor/vql"
	"www.velocidex.com/golang/velociraptor/vql/tools"
	vfilter "www.velocidex.com/golang/vfilter"
)

var (
	golden_command = app.Command(
		"golden", "Run tests and compare against golden files.")

	golden_command_prefix = golden_command.Arg(
		"prefix", "Golden file prefix").Required().String()

	testonly = golden_command.Flag("testonly", "Do not update the fixture.").Bool()
)

type testFixture struct {
	Parameters map[string]string `json:"Parameters"`
	Queries    []string          `json:"Queries"`
}

// We want to emulate as closely as possible the logic in the artifact
// collector client action. Therefore we build a vql_collector_args
// from the fixture.
func vqlCollectorArgsFromFixture(
	config_obj *config_proto.Config,
	fixture *testFixture) *actions_proto.VQLCollectorArgs {
	artifact_collector_args := &flows_proto.ArtifactCollectorArgs{
		Parameters: &flows_proto.ArtifactParameters{},
	}

	for k, v := range fixture.Parameters {
		artifact_collector_args.Parameters.Env = append(
			artifact_collector_args.Parameters.Env,
			&actions_proto.VQLEnv{Key: k, Value: v})
	}

	vql_collector_args := &actions_proto.VQLCollectorArgs{}
	err := artifacts.AddArtifactCollectorArgs(
		config_obj,
		vql_collector_args,
		artifact_collector_args)
	kingpin.FatalIfError(err, "vqlCollectorArgsFromFixture")

	return vql_collector_args
}

func runTest(fixture *testFixture) (string, error) {
	config_obj, err := DefaultConfigLoader.LoadAndValidate()
	kingpin.FatalIfError(err, "Unable to load config file")

	err = services.StartJournalService(config_obj)
	kingpin.FatalIfError(err, "Unable to start services")

	// Create an output container.
	tmpfile, err := ioutil.TempFile("", "golden")
	if err != nil {
		log.Fatal(err)
	}

	container, err := reporting.NewContainer(tmpfile.Name())
	kingpin.FatalIfError(err, "Can not create output container")

	builder := artifacts.ScopeBuilder{
		Config:     config_obj,
		ACLManager: vql_subsystem.NewRoleACLManager("administrator"),
		Logger:     log.New(&LogWriter{config_obj}, "Velociraptor: ", log.Lshortfile),
		Uploader:   container,
		Env: ordereddict.NewDict().
			Set("GoldenOutput", tmpfile.Name()).
			Set(constants.SCOPE_MOCK, &tools.MockingScopeContext{}),
	}

	if env_map != nil {
		for k, v := range *env_map {
			builder.Env.Set(k, v)
		}
	}

	vql_collector_args := vqlCollectorArgsFromFixture(config_obj, fixture)
	for _, env_spec := range vql_collector_args.Env {
		builder.Env.Set(env_spec.Key, env_spec.Value)
	}

	// Cleanup after the query.
	scope := builder.BuildFromScratch()
	defer scope.Close()

	scope.AddDestructor(func() {
		container.Close()
		os.Remove(tmpfile.Name()) // clean up
	})

	result := ""
	for _, query := range fixture.Queries {
		result += query
		scope.Log("Running query %v", query)
		vql, err := vfilter.Parse(query)
		if err != nil {
			return "", err
		}

		result_chan := vfilter.GetResponseChannel(
			vql, context.Background(), scope, 1000, 1000)
		for {
			query_result, ok := <-result_chan
			if !ok {
				break
			}
			result += string(query_result.Payload)
		}
	}

	return result, nil
}

func doGolden() {
	globs, err := filepath.Glob(fmt.Sprintf(
		"%s*.in.yaml", *golden_command_prefix))
	kingpin.FatalIfError(err, "Glob")

	logger := log.New(os.Stderr, "golden: ", log.Lshortfile)

	failures := []string{}

	for _, filename := range globs {
		logger.Printf("Openning %v", filename)
		data, err := ioutil.ReadFile(filename)
		kingpin.FatalIfError(err, "Reading file")

		fixture := testFixture{}
		err = yaml.Unmarshal(data, &fixture)
		kingpin.FatalIfError(err, "Unmarshal input file")

		result, err := runTest(&fixture)
		kingpin.FatalIfError(err, "Running test")

		outfile := strings.Replace(filename, ".in.", ".out.", -1)
		old_data, err := ioutil.ReadFile(outfile)
		if err == nil {
			if strings.TrimSpace(string(old_data)) != strings.TrimSpace(result) {
				dmp := diffmatchpatch.New()
				diffs := dmp.DiffMain(
					string(old_data), result, false)
				fmt.Printf("Failed %v:\n", filename)
				fmt.Println(dmp.DiffPrettyText(diffs))
				failures = append(failures, filename)
			}
		} else {
			fmt.Printf("New file for  %v:\n", filename)
			fmt.Println(result)
			failures = append(failures, filename)
		}

		if *testonly {
			continue
		}

		err = ioutil.WriteFile(
			outfile,
			[]byte(result), 0666)
		kingpin.FatalIfError(err, "Unable to write golden file")
	}

	if len(failures) > 0 {
		kingpin.Fatalf(
			"Failed! Some golden files did not match:%s\n",
			failures)
	}
}

func init() {
	command_handlers = append(command_handlers, func(command string) bool {
		switch command {
		case "golden":
			doGolden()
		default:
			return false
		}
		return true
	})
}
