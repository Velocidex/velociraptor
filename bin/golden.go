/*
Velociraptor - Dig Deeper
Copyright (C) 2019-2025 Rapid7 Inc.

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
	"regexp"
	"runtime/pprof"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/Velocidex/ordereddict"
	errors "github.com/go-errors/errors"
	"github.com/mccutchen/go-httpbin/v2/httpbin"
	"github.com/sergi/go-diff/diffmatchpatch"
	proto "google.golang.org/protobuf/proto"
	"gopkg.in/yaml.v3"
	"www.velocidex.com/golang/velociraptor/actions"
	actions_proto "www.velocidex.com/golang/velociraptor/actions/proto"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	"www.velocidex.com/golang/velociraptor/constants"
	"www.velocidex.com/golang/velociraptor/crypto/storage"
	crypto_utils "www.velocidex.com/golang/velociraptor/crypto/utils"
	"www.velocidex.com/golang/velociraptor/file_store"
	"www.velocidex.com/golang/velociraptor/json"
	logging "www.velocidex.com/golang/velociraptor/logging"
	"www.velocidex.com/golang/velociraptor/paths"
	"www.velocidex.com/golang/velociraptor/reporting"
	"www.velocidex.com/golang/velociraptor/services"
	"www.velocidex.com/golang/velociraptor/services/writeback"
	"www.velocidex.com/golang/velociraptor/startup"
	"www.velocidex.com/golang/velociraptor/utils"
	"www.velocidex.com/golang/velociraptor/utils/tempfile"
	vql_subsystem "www.velocidex.com/golang/velociraptor/vql"
	"www.velocidex.com/golang/velociraptor/vql/acl_managers"
	"www.velocidex.com/golang/velociraptor/vql/psutils"
	"www.velocidex.com/golang/velociraptor/vql/remapping"
	"www.velocidex.com/golang/velociraptor/vtesting"
	vfilter "www.velocidex.com/golang/vfilter"
	"www.velocidex.com/golang/vfilter/arg_parser"
)

var (
	golden_command = app.Command(
		"golden", "Run tests and compare against golden files.")

	golden_command_directory = golden_command.Arg(
		"directory", "Golden file directory path").Required().String()

	golden_command_filter = golden_command.Flag("filter", "A regex to filter the test files").
				String()

	golden_env_map = golden_command.Flag("env", "Environment for the query.").
			StringMap()

	testonly      = golden_command.Flag("testonly", "Do not update the fixture.").Bool()
	disable_alarm = golden_command.Flag("disable_alarm", "Do not terminate when deadlocked.").Bool()

	golden_update_datastore = golden_command.Flag("update_datastore",
		"Normally golden tests run with the readonly datastore so as not to "+
			"change the fixture. This flag allows updates to the fixtures.").
		Bool()

	// If the logs emit messages matching these then the test is
	// considered failed. This helps us catch VQL errors.
	fatalLogMessagesRegex = []string{
		"(?i)Symbol .+ not found",
		"(?i)Field .+ Expecting a .+ arg type, not",
		"(?i)Artifact .+ not found",
		"(?i)Order by column .+ not present in row",
		"PANIC runtime error:",
		"Extra unrecognized arg",
	}
)

type queryDesc struct {
	Comment string
	Query   string
}

type testFixture struct {
	Parameters    map[string]string
	Queries       []queryDesc
	ConfigPatches []string
	ConfigMerges  []string
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

func makeCtxWithTimeout(
	root_ctx context.Context, duration int) (context.Context, func()) {
	ctx, cancel := context.WithCancel(root_ctx)

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
					pid := int32(os.Getpid())
					total_time, _ := psutils.TimesWithContext(ctx, pid)
					memory, _ := psutils.MemoryInfoWithContext(ctx, pid)

					fmt.Printf("Not time to fire yet %v %v %v\n",
						time.Now(), json.MustMarshalString(total_time),
						json.MustMarshalString(memory))
					continue
				}

				p := pprof.Lookup("goroutine")
				if p != nil {
					_ = p.WriteTo(os.Stdout, 1)
				}

				p = pprof.Lookup("mutex")
				if p != nil {
					_ = p.WriteTo(os.Stdout, 1)
				}

				// Write the recent queries.
				fmt.Println("Recent Queries.")
				for _, q := range actions.QueryLog.Get() {
					fmt.Println(q)
				}

				os.Stdout.Close()

				// Hard exit with an error.
				os.Exit(-1)
			}
		}
	}()

	return ctx, cancel
}

func runTest(fixture *testFixture, sm *services.Service,
	config_obj *config_proto.Config) (string, error) {

	config_obj = proto.Clone(config_obj).(*config_proto.Config)

	err := applyMergesAndPatches(config_obj,
		nil, fixture.ConfigMerges,
		nil, fixture.ConfigPatches)
	if err != nil {
		return "", err
	}

	gen := utils.IncrementalIdGenerator(0)
	defer utils.SetIdGenerator(&gen)()

	// Freeze the time for consistent golden tests Monday, May 31, 2020 3:28:05 PM
	closer := utils.MockTime(utils.NewMockClock(time.Unix(1590938885, 10)))
	defer closer()

	ctx := sm.Ctx

	// Limit each test for maxmimum time
	if !*disable_alarm {
		sub_ctx, cancel := makeCtxWithTimeout(ctx, 30)
		defer cancel()

		ctx = sub_ctx
	}

	// Set this to emulate a working client.
	storage.SetCurrentServerPem([]byte(config_obj.Frontend.Certificate))

	writeback_service := writeback.GetWritebackService()
	err = writeback_service.LoadWriteback(config_obj)
	if err != nil {
		log.Fatal(err)
	}

	err = crypto_utils.VerifyConfig(config_obj)
	if err != nil {
		log.Fatal(err)
	}

	// Create an output container.
	tmpfile, err := tempfile.TempFile("golden")
	if err != nil {
		log.Fatal(err)
	}
	defer os.Remove(tmpfile.Name())

	container, err := reporting.NewContainer(
		config_obj, tmpfile.Name(), "", 5, nil)
	if err != nil {
		return "", fmt.Errorf("Can not create output container: %w", err)
	}
	log_writer.Clear()

	builder := services.ScopeBuilder{
		Config:     config_obj,
		ACLManager: acl_managers.NewRoleACLManager(config_obj, "administrator", "org_admin"),
		Logger:     log.New(log_writer, "Velociraptor: ", 0),
		Uploader:   container,
		Env: ordereddict.NewDict().
			Set("GoldenOutput", tmpfile.Name()).
			Set("config", config_obj.Client).
			Set("_SessionId", "F.Golden").
			Set(constants.SCOPE_MOCK, &remapping.MockingScopeContext{}),
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
	manager, err := services.GetRepositoryManager(config_obj)
	if err != nil {
		return "", err
	}
	scope := manager.BuildScopeFromScratch(builder)
	defer scope.Close()

	err = scope.AddDestructor(func() {
		container.Close()
		os.Remove(tmpfile.Name()) // clean up
	})
	if err != nil {
		return "", err
	}

	// Build the result of the query into a golden file.
	result := ""
	for _, query := range fixture.Queries {
		if query.Comment != "" {
			result += query.Comment + "\n"
		}
		result += fmt.Sprintf("Query: %v\n", query.Query)
		scope.Log("Running query %v", query.Query)
		vql, err := vfilter.Parse(query.Query)
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
			result += fmt.Sprintf("Output: %v\n\n", string(query_result.Payload))
		}
	}

	for _, msg := range fatalLogMessagesRegex {
		matches, err := log_writer.Matches(msg)
		if matches || err != nil {
			return "", fmt.Errorf("Log out matches %q", msg)
		}
	}

	return result, nil
}

func doGolden() error {
	closer := utils.MockTime(utils.NewMockClock(time.Unix(1590938885, 10)))
	defer closer()

	logging.DisableLogging()
	services.AllowFrontendPlugins.Store(true)

	vql_subsystem.RegisterPlugin(&MemoryLogPlugin{})
	vql_subsystem.RegisterFunction(&WriteFilestoreFunction{})
	vql_subsystem.RegisterFunction(&MockTimeFunciton{})
	vql_subsystem.RegisterFunction(&HTTPBinFunction{})

	config_obj, err := makeDefaultConfigLoader().LoadAndValidate()
	if err != nil {
		return err
	}

	// Do not update the datastore - this allows golden tests to avoid
	// modifying the fixtures.
	if !*golden_update_datastore {
		config_obj.Datastore.Implementation = "ReadOnlyDataStore"
	}

	logger := logging.GetLogger(config_obj, &logging.ToolComponent)
	logger.Info("Starting golden file test.")
	log_writer = &MemoryLogWriter{config_obj: config_obj}

	failures := []string{}

	config_obj.Services = services.GoldenServicesSpec()

	ctx, cancel := install_sig_handler()
	defer cancel()

	// Global timeout for the entire test
	if !*disable_alarm {
		timeout_ctx, cancel := makeCtxWithTimeout(ctx, 120)
		defer cancel()

		ctx = timeout_ctx
	}

	sm, err := startup.StartToolServices(ctx, config_obj)
	defer sm.Close()

	if err != nil {
		return err
	}

	var file_paths []string

	err = filepath.Walk(*golden_command_directory, func(file_path string, info os.FileInfo, err error) error {
		if *golden_command_filter != "" &&
			!strings.HasPrefix(filepath.Base(file_path), *golden_command_filter) {
			return nil
		}

		if !strings.HasSuffix(file_path, ".in.yaml") {
			return nil
		}

		file_paths = append(file_paths, file_path)
		return nil
	})

	// Run the test cases in a predictable way
	sort.Strings(file_paths)
	logger.Info("<green>Testing %v test cases</>", len(file_paths))

	for _, file_path := range file_paths {
		select {
		case <-sm.Ctx.Done():
			return errors.New("Cancelled!")
		default:
		}

		logger := log.New(os.Stderr, "golden: ", 0)

		logger.Printf("Opening %v", file_path)
		data, err := ioutil.ReadFile(file_path)
		if err != nil {
			return fmt.Errorf("Reading file: %w", err)
		}

		fixture, err := parseFixture(data)
		if err != nil {
			return fmt.Errorf("Unmarshal input file: %w", err)
		}

		result, err := runTest(&fixture, sm, config_obj)
		if err != nil {
			return fmt.Errorf("Running test %v: %w", file_path, err)
		}

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
			if err != nil {
				return fmt.Errorf("Unable to write golden file: %w", err)
			}
		}
	}

	if err != nil {
		return fmt.Errorf("golden error FAIL: %w", err)
	}

	if len(failures) > 0 {
		return fmt.Errorf(
			"Failed! Some golden files did not match: %s\n", failures)
	}
	return nil
}

func init() {
	command_handlers = append(command_handlers, func(command string) bool {
		switch command {
		case golden_command.FullCommand():
			FatalIfError(golden_command, doGolden)

		default:
			return false
		}
		return true
	})
}

var log_writer *MemoryLogWriter

type MemoryLogWriter struct {
	mu         sync.Mutex
	config_obj *config_proto.Config
	logs       []string
}

func (self *MemoryLogWriter) Clear() {
	self.mu.Lock()
	defer self.mu.Unlock()

	self.logs = nil
}

func (self *MemoryLogWriter) Write(b []byte) (int, error) {
	self.mu.Lock()
	defer self.mu.Unlock()

	self.logs = append(self.logs, string(b))

	logging.GetLogger(self.config_obj, &logging.ClientComponent).Info("%v", string(b))
	return len(b), nil
}

func (self *MemoryLogWriter) Matches(pattern string) (bool, error) {
	self.mu.Lock()
	defer self.mu.Unlock()

	re, err := regexp.Compile(pattern)
	if err != nil {
		return false, err
	}

	for _, line := range self.logs {
		if re.FindString(line) != "" {
			return true, nil
		}
	}

	return false, nil
}

// Some tests need to inspect the logs
type MemoryLogPlugin struct{}

func (self MemoryLogPlugin) Call(
	ctx context.Context,
	scope vfilter.Scope,
	args *ordereddict.Dict) <-chan vfilter.Row {
	output_chan := make(chan vfilter.Row)

	go func() {
		defer close(output_chan)

		if log_writer != nil {
			for _, line := range log_writer.logs {
				output_chan <- ordereddict.NewDict().
					Set("Log", line)
			}

			log_writer.Clear()
		}

	}()

	return output_chan
}

func (self MemoryLogPlugin) Info(scope vfilter.Scope, type_map *vfilter.TypeMap) *vfilter.PluginInfo {
	return &vfilter.PluginInfo{
		Name:    "test_read_logs",
		Doc:     "Read logs in golden test.",
		ArgType: type_map.AddType(scope, vfilter.Null{}),
	}
}

type WriteFilestoreFunctionArgs struct {
	Data   string `vfilter:"optional,field=data"`
	FSPath string `vfilter:"optional,field=path"`
}

type WriteFilestoreFunction struct{}

func (self WriteFilestoreFunction) Call(ctx context.Context,
	scope vfilter.Scope,
	args *ordereddict.Dict) vfilter.Any {

	arg := &WriteFilestoreFunctionArgs{}
	err := arg_parser.ExtractArgsWithContext(ctx, scope, args, arg)
	if err != nil {
		scope.Log("write_filestore: %s", err)
		return &vfilter.Null{}
	}

	config_obj, ok := vql_subsystem.GetServerConfig(scope)
	if !ok {
		scope.Log("Command can only run on the server")
		return &vfilter.Null{}
	}

	file_store_factory := file_store.GetFileStore(config_obj)
	pathspec := paths.FSPathSpecFromClientPath(arg.FSPath)
	writer, err := file_store_factory.WriteFile(pathspec)
	if err != nil {
		scope.Log("write_filestore: %v", err)
		return &vfilter.Null{}
	}
	defer writer.Close()

	_, err = writer.Write([]byte(arg.Data))
	if err != nil {
		scope.Log("write_filestore: %v", err)
		return &vfilter.Null{}
	}

	return true
}

func (self WriteFilestoreFunction) Info(
	scope vfilter.Scope, type_map *vfilter.TypeMap) *vfilter.FunctionInfo {
	return &vfilter.FunctionInfo{
		Name:    "write_filestore",
		Doc:     "Write a file on the filestore.",
		ArgType: type_map.AddType(scope, &WriteFilestoreFunctionArgs{}),
	}
}

type MockTimeFuncitonArgs struct {
	Now int64 `vfilter:"required,field=now"`
}

type MockTimeFunciton struct{}

func (self MockTimeFunciton) Call(ctx context.Context,
	scope vfilter.Scope,
	args *ordereddict.Dict) vfilter.Any {

	arg := &MockTimeFuncitonArgs{}
	err := arg_parser.ExtractArgsWithContext(ctx, scope, args, arg)
	if err != nil {
		scope.Log("mock_time: %s", err)
		return &vfilter.Null{}
	}

	clock := utils.NewMockClock(time.Unix(arg.Now, 0))
	cancel := utils.MockTime(clock)
	err = vql_subsystem.GetRootScope(scope).AddDestructor(cancel)
	if err != nil {
		scope.Log("mock_time: %s", err)
		return &vfilter.Null{}
	}

	return true
}

func (self MockTimeFunciton) Info(
	scope vfilter.Scope, type_map *vfilter.TypeMap) *vfilter.FunctionInfo {
	return &vfilter.FunctionInfo{
		Name:    "mock_time",
		ArgType: type_map.AddType(scope, &MockTimeFuncitonArgs{}),
	}
}

func parseFixture(data []byte) (res testFixture, err error) {
	type tmpType struct {
		ConfigMerges  []string          `yaml:"ConfigMerges"`
		ConfigPatches []string          `yaml:"ConfigPatches"`
		Parameters    map[string]string `yaml:"Parameters"`
		Queries       []yaml.Node       `yaml:"Queries"`
	}

	var n tmpType
	err = yaml.Unmarshal(data, &n)
	if err != nil {
		return res, err
	}

	res.ConfigMerges = n.ConfigMerges
	res.ConfigPatches = n.ConfigPatches
	res.Parameters = n.Parameters
	for _, node := range n.Queries {
		if node.Kind != yaml.ScalarNode {
			continue
		}

		res.Queries = append(res.Queries, queryDesc{
			Query:   node.Value,
			Comment: node.HeadComment,
		})
	}

	return res, err
}

// A helper function to spin up go-httpbin for testing http
// connections.
type HTTPBinFunction struct{}

func (self HTTPBinFunction) Call(ctx context.Context,
	scope vfilter.Scope,
	args *ordereddict.Dict) vfilter.Any {

	app := httpbin.New()
	testServer := vtesting.NewServer(app, 8006)

	err := vql_subsystem.GetRootScope(scope).AddDestructor(testServer.Close)
	if err != nil {
		scope.Log("httpbin: %v", err)
	}
	return testServer.URL
}

func (self HTTPBinFunction) Info(
	scope vfilter.Scope, type_map *vfilter.TypeMap) *vfilter.FunctionInfo {
	return &vfilter.FunctionInfo{
		Name: "httpbin",
		Doc:  "Start HTTPBin and return the URL",
	}
}
