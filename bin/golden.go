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
	"runtime/pprof"
	"strings"
	"time"

	"github.com/Velocidex/ordereddict"
	"github.com/Velocidex/yaml/v2"
	"github.com/sergi/go-diff/diffmatchpatch"
	"github.com/shirou/gopsutil/process"
	kingpin "gopkg.in/alecthomas/kingpin.v2"
	actions_proto "www.velocidex.com/golang/velociraptor/actions/proto"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	"www.velocidex.com/golang/velociraptor/constants"
	logging "www.velocidex.com/golang/velociraptor/logging"
	"www.velocidex.com/golang/velociraptor/reporting"
	"www.velocidex.com/golang/velociraptor/services"
	"www.velocidex.com/golang/velociraptor/startup"
	vql_subsystem "www.velocidex.com/golang/velociraptor/vql"
	"www.velocidex.com/golang/velociraptor/vql/tools"
	vfilter "www.velocidex.com/golang/vfilter"
)

var (
	golden_command = app.Command(
		"golden", "Run tests and compare against golden files.")

	golden_command_prefix = golden_command.Arg(
		"prefix", "Golden file prefix").Required().String()

	golden_env_map = golden_command.Flag("env", "Environment for the query.").
			StringMap()

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

	vql_collector_args := &actions_proto.VQLCollectorArgs{}
	for k, v := range fixture.Parameters {
		vql_collector_args.Env = append(vql_collector_args.Env,
			&actions_proto.VQLEnv{Key: k, Value: v})
	}

	return vql_collector_args
}

func makeCtxWithTimeout(duration int) (context.Context, func()) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Second*60)

	deadline := time.Now().Add(time.Second * time.Duration(duration))
	fmt.Printf("Setting deadline to %v\n", deadline)

	// Set an alarm for hard exit in 2 minutes. If we hit it then
	// the code is deadlocked and we want to know what is
	// happening.
	go func() {
		for {
			select {
			case <-ctx.Done():
				fmt.Printf("Disarming alarm\n")
				return

				// If we get here we are deadlocked! Print all
				// the goroutines and mutex and hard exit.
			case <-time.After(time.Second):
				if time.Now().Before(deadline) {
					proc, _ := process.NewProcess(int32(os.Getpid()))
					total_time, _ := proc.Percent(0)
					memory, _ := proc.MemoryInfo()

					fmt.Printf("Not time to fire yet %v %v %v\n",
						time.Now(), total_time, memory)
					continue
				}

				p := pprof.Lookup("goroutine")
				if p != nil {
					p.WriteTo(os.Stdout, 1)
				}

				p = pprof.Lookup("mutex")
				if p != nil {
					p.WriteTo(os.Stdout, 1)
				}

				os.Stdout.Close()

				// Hard exit with an error.
				os.Exit(-1)
			}
		}
	}()

	return ctx, cancel
}

func runTest(fixture *testFixture,
	config_obj *config_proto.Config) (string, error) {

	ctx, cancel := makeCtxWithTimeout(30)
	defer cancel()

	//Force a clean slate for each test.
	startup.Reset()

	sm, err := startEssentialServices(config_obj)
	if err != nil {
		return "", err
	}
	defer sm.Close()

	_, err = getRepository(config_obj)
	kingpin.FatalIfError(err, "Loading extra artifacts")

	// Create an output container.
	tmpfile, err := ioutil.TempFile("", "golden")
	if err != nil {
		log.Fatal(err)
	}
	defer os.Remove(tmpfile.Name())

	container, err := reporting.NewContainer(tmpfile.Name())
	kingpin.FatalIfError(err, "Can not create output container")

	builder := services.ScopeBuilder{
		Config:     config_obj,
		ACLManager: vql_subsystem.NewRoleACLManager("administrator"),
		Logger:     log.New(&LogWriter{config_obj}, "Velociraptor: ", 0),
		Uploader:   container,
		Env: ordereddict.NewDict().
			Set("GoldenOutput", tmpfile.Name()).
			Set(constants.SCOPE_MOCK, &tools.MockingScopeContext{}),
	}

	if golden_env_map != nil {
		for k, v := range *golden_env_map {
			builder.Env.Set(k, v)
		}
	}

	vql_collector_args := vqlCollectorArgsFromFixture(config_obj, fixture)
	for _, env_spec := range vql_collector_args.Env {
		builder.Env.Set(env_spec.Key, env_spec.Value)
	}

	// Cleanup after the query.
	manager, err := services.GetRepositoryManager()
	kingpin.FatalIfError(err, "GetRepositoryManager")
	scope := manager.BuildScopeFromScratch(builder)
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
			vql, ctx, scope,
			vql_subsystem.MarshalJsonIndent(scope),
			1000, 1000)
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
	_, cancel := makeCtxWithTimeout(120)
	defer cancel()

	config_obj, err := DefaultConfigLoader.LoadAndValidate()
	kingpin.FatalIfError(err, "Can not load configuration.")

	logger := logging.GetLogger(config_obj, &logging.ToolComponent)
	logger.Info("Starting golden file test.")

	failures := []string{}

	err = filepath.Walk(*golden_command_prefix, func(file_path string, info os.FileInfo, err error) error {

		if !strings.HasSuffix(file_path, ".in.yaml") {
			return nil
		}

		logger := log.New(os.Stderr, "golden: ", 0)

		logger.Printf("Opening %v", file_path)
		data, err := ioutil.ReadFile(file_path)
		kingpin.FatalIfError(err, "Reading file")

		fixture := testFixture{}
		err = yaml.Unmarshal(data, &fixture)
		kingpin.FatalIfError(err, "Unmarshal input file")

		result, err := runTest(&fixture, config_obj)
		kingpin.FatalIfError(err, "Running test")

		outfile := strings.Replace(file_path, ".in.", ".out.", -1)
		old_data, err := ioutil.ReadFile(outfile)
		if err == nil {
			if strings.TrimSpace(string(old_data)) != strings.TrimSpace(result) {
				dmp := diffmatchpatch.New()
				diffs := dmp.DiffMain(
					string(old_data), result, false)
				fmt.Printf("Failed %v:\n", file_path)
				fmt.Println(dmp.DiffPrettyText(diffs))

				failures = append(failures, file_path)
			}
		} else {
			fmt.Printf("New file for  %v:\n", file_path)
			fmt.Println(result)

			failures = append(failures, file_path)
		}

		if !*testonly {
			err = ioutil.WriteFile(
				outfile,
				[]byte(result), 0666)
			kingpin.FatalIfError(err, "Unable to write golden file")
		}
		return nil
	})

	if err != nil {
		kingpin.Fatalf("golden error: %s", err)
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
