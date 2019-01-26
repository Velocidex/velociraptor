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
	"path/filepath"
	"strings"

	"github.com/Velocidex/yaml"
	"github.com/sergi/go-diff/diffmatchpatch"
	kingpin "gopkg.in/alecthomas/kingpin.v2"
	actions_proto "www.velocidex.com/golang/velociraptor/actions/proto"
	api_proto "www.velocidex.com/golang/velociraptor/api/proto"
	artifacts "www.velocidex.com/golang/velociraptor/artifacts"
	"www.velocidex.com/golang/velociraptor/flows"
	flows_proto "www.velocidex.com/golang/velociraptor/flows/proto"
	logging "www.velocidex.com/golang/velociraptor/logging"
	vql_subsystem "www.velocidex.com/golang/velociraptor/vql"
	vfilter "www.velocidex.com/golang/vfilter"
)

var (
	golden_command = app.Command(
		"golden", "Run tests and compare against golden files.")

	golden_command_prefix = golden_command.Arg(
		"prefix", "Golden file prefix").Required().String()
)

type testFixture struct {
	Parameters map[string]string `json:"Parameters"`
	Files      map[string]string `json:"Files"`
	Queries    []string          `json:"Queries"`
}

// We want to emulate as closely as possible the logic in the artifact
// collector client action. Therefore we build a vql_collector_args
// from the fixture.
func vqlCollectorArgsFromFixture(
	config_obj *api_proto.Config,
	fixture *testFixture) *actions_proto.VQLCollectorArgs {
	artifact_collector_args := &flows_proto.ArtifactCollectorArgs{
		Parameters: &flows_proto.ArtifactParameters{},
	}

	for k, v := range fixture.Parameters {
		artifact_collector_args.Parameters.Env = append(
			artifact_collector_args.Parameters.Env,
			&actions_proto.VQLEnv{Key: k, Value: v})
	}
	for k, v := range fixture.Files {
		artifact_collector_args.Parameters.Files = append(
			artifact_collector_args.Parameters.Files,
			&actions_proto.VQLEnv{Key: k, Value: v})
	}

	vql_collector_args := &actions_proto.VQLCollectorArgs{}
	err := flows.AddArtifactCollectorArgs(
		config_obj,
		vql_collector_args,
		artifact_collector_args)
	kingpin.FatalIfError(err, "vqlCollectorArgsFromFixture")

	return vql_collector_args
}

func runTest(fixture *testFixture) (string, error) {
	config_obj := get_config_or_default()
	repository := getRepository(config_obj)

	env := vfilter.NewDict().
		Set("config", config_obj.Client).
		Set("server_config", config_obj).
		Set(vql_subsystem.CACHE_VAR, vql_subsystem.NewScopeCache())

	scope := artifacts.MakeScope(repository).AppendVars(env)
	defer scope.Close()

	scope.Logger = logging.NewPlainLogger(config_obj,
		&logging.ToolComponent)

	vql_collector_args := vqlCollectorArgsFromFixture(
		config_obj, fixture)
	for _, env_spec := range vql_collector_args.Env {
		env.Set(env_spec.Key, env_spec.Value)
	}

	result := ""
	for _, query := range fixture.Queries {
		result += query
		vql, err := vfilter.Parse(query)
		if err != nil {
			return "", err
		}

		result_chan := vfilter.GetResponseChannel(
			vql, context.Background(), scope, 1000, 100)
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

	failures := []string{}

	for _, filename := range globs {
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
			if string(old_data) != result {
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
