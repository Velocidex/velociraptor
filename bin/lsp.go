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
	"io"
	"net"
	"os"
	"path/filepath"

	"github.com/alecthomas/kingpin/v2"
	errors "github.com/go-errors/errors"
	"go.lsp.dev/jsonrpc2"
	"go.lsp.dev/protocol"
	"www.velocidex.com/golang/velociraptor/api"
	api_proto "www.velocidex.com/golang/velociraptor/api/proto"
	"www.velocidex.com/golang/velociraptor/config"
	"www.velocidex.com/golang/velociraptor/constants"
	"www.velocidex.com/golang/velociraptor/grpc_client"
	"www.velocidex.com/golang/velociraptor/logging"
	"www.velocidex.com/golang/velociraptor/services"
	"www.velocidex.com/golang/velociraptor/startup"
	"www.velocidex.com/golang/velociraptor/utils"
	"www.velocidex.com/golang/velociraptor/utils/tempfile"
	vql_subsystem "www.velocidex.com/golang/velociraptor/vql"
	"www.velocidex.com/golang/velociraptor/vql/lsp"
)

var (
	lsp_cmd = app.Command("lsp",
		"Run a VQL language server on stdin/stdout (LSP over stdio).")

	lsp_command_datastore = lsp_cmd.Flag(
		"datastore", "Path to a datastore directory (defaults to temp)").
		ExistingDir()

	lsp_log_file = lsp_cmd.Flag("log", "Write the LSP log to a file instead of stdout").
			String()

	lsp_check_plugin = lsp_cmd.Flag("check", "Check a VQL query and exit").
				String()
)

func init() {
	command_handlers = append(command_handlers, func(command string) bool {
		switch command {
		case lsp_cmd.FullCommand():
			FatalIfError(lsp_cmd, doLSP)
		default:
			return false
		}
		return true
	})
}

func doLSP() error {
	logging.DisableLogging()

	// Start from a clean slate
	os.Setenv(constants.VELOCIRAPTOR_CONFIG, "")
	os.Setenv(constants.VELOCIRAPTOR_LITERAL_CONFIG, "")

	// The lsp command behaves like the gui/collector commands: it may
	// be pointed at an existing datastore (in which case it loads the
	// config from there) or it will generate a fresh local config in a
	// temp directory. This gives the language server access to custom
	// artifacts stored in the repository.
	datastore_directory := *lsp_command_datastore
	if datastore_directory == "" {
		datastore_directory = utils.Join(tempfile.GetTempDir(), "gui_datastore")
		// Ensure the directory exists
		err := os.MkdirAll(datastore_directory, 0o777)
		if err != nil {
			return fmt.Errorf("Unable to create datastore directory: %w", err)
		}
	}

	datastore_directory, err := filepath.Abs(datastore_directory)
	if err != nil {
		return fmt.Errorf("Unable find path: %w", err)
	}

	server_config_path := utils.Join(datastore_directory, "server.config.yaml")
	client_config_path := utils.Join(datastore_directory, "client.config.yaml")

	// Try to open the config file from there
	config_obj, err := makeDefaultConfigLoader().
		WithVerbose(*verbose_flag).
		WithFileLoader(server_config_path).LoadAndValidate()
	if err != nil || config_obj.Frontend == nil {
		// Stop on hard errors but if the file does not exist we need
		// to create it below..
		hard_err, ok := err.(config.HardError)
		if ok && !errors.Is(hard_err.Err, os.ErrNotExist) {
			return err
		}

		// Need to generate a new config. This config is the same as
		// the `gui` command makes so the same datastore can be shared.
		logging.Prelog("No valid config found - "+
			"will generate a new one at <green>%s</>", server_config_path)

		config_obj, err = generateGUIConfig(
			datastore_directory, server_config_path, client_config_path)
		if err != nil {
			return err
		}
	}

	if config_obj.Services == nil {
		config_obj.Services = services.GenericToolServices()
	}

	// The lsp command starts a local API server to fetch artifacts.
	// If the default API port (8001) is already in use — for example
	// when a production Velociraptor server is running on the same
	// machine — we automatically pick a free port instead. The
	// superuser client below connects to whatever port we settle on.
	if config_obj.API == nil {
		return errors.New("API server not configured")
	}
	bind_addr := fmt.Sprintf("%s:%d",
		config_obj.API.BindAddress, config_obj.API.BindPort)
	lis, err := net.Listen("tcp", bind_addr)
	if err != nil {
		// The default port is taken. Grab a free ephemeral port on
		// the same address and use that instead.
		lis, err = net.Listen("tcp",
			fmt.Sprintf("%s:0", config_obj.API.BindAddress))
		if err != nil {
			return fmt.Errorf("unable to find a free API port: %w", err)
		}
		config_obj.API.BindPort = uint32(
			lis.Addr().(*net.TCPAddr).Port)
		logging.Prelog("API port 8001 is in use - "+
			"using free port %d instead", config_obj.API.BindPort)
	}
	lis.Close()

	ctx, cancel := install_sig_handler()
	defer cancel()

	sm, err := startup.StartToolServices(ctx, config_obj)
	if err != nil {
		return fmt.Errorf("starting tool services: %w", err)
	}
	defer sm.Close()

	// Start a local gRPC API server. Like the gui command we start
	// the API service so we can query it as the superuser. This is
	// the "hybrid" mode: the lsp server is an API client, which gives
	// it access to the full repository (including custom artifacts)
	// through the same code paths used by the API.
	server_builder, err := api.NewServerBuilder(sm.Ctx, config_obj, sm.Wg)
	if err != nil {
		return err
	}

	err = server_builder.WithAPIServer(sm.Ctx, sm.Wg)
	if err != nil {
		return err
	}

	// Connect to the local API server with the superuser identity. No
	// authentication is needed for local calls since we present the
	// frontend certificate.
	api_client, closer, err := grpc_client.Factory.GetAPIClient(
		ctx, grpc_client.SuperUser, config_obj)
	if err != nil {
		return err
	}
	defer closer()

	// If we are asked to check a query, do it and exit. This is useful
	// for testing and for agents that just want validation.
	if *lsp_check_plugin != "" {
		return checkQuery(ctx, api_client, *lsp_check_plugin)
	}

	registry, err := buildRegistry(ctx, api_client)
	if err != nil {
		return err
	}

	// The LSP protocol runs over stdio. It is important that nothing
	// else writes to stdout, so we may redirect logging elsewhere.
	var in io.Reader = os.Stdin
	var out io.Writer = os.Stdout
	if *lsp_log_file != "" {
		fd, err := os.Create(*lsp_log_file)
		if err != nil {
			return err
		}
		defer fd.Close()

		// If a log file is given, redirect all server messages there.
		out = io.MultiWriter(os.Stdout, fd)
	}

	stdio := stdioConn{
		reader: in,
		writer: out,
	}
	defer stdio.Close()

	stream := jsonrpc2.NewStream(stdio)

	server := lsp.NewServer(registry)

	// NewServer wires the stdio stream to the server and starts the
	// read loop in a background goroutine; it does not block. We need
	// to block until the client closes the connection or asks to exit.
	_, conn, _ := protocol.NewServer(context.Background(), server, stream)

	// Block until the connection is closed by the client or the client
	// sends shutdown/exit (which closes server.Done()).
	select {
	case <-conn.Done():
	case <-server.Done():
	}

	// The process terminates right after this function returns, so we
	// do not need to wait for the read loop to unwind. conn.Close()
	// would block on the read goroutine stuck at os.Stdin; skip it and
	// let the OS clean up.
	return nil
}

// buildRegistry loads the VQL scope and registers all plugins, functions
// and artifacts. Artifacts are fetched over the API so the registry
// matches what the repository actually contains.
func buildRegistry(
	ctx context.Context, api_client api_proto.APIClient) (*lsp.Registry, error) {

	scope := vql_subsystem.MakeScope()
	defer scope.Close()

	registry, err := lsp.BuildRegistryFromAPIClient(ctx, api_client, scope)
	if err != nil {
		return nil, err
	}

	return registry, nil
}

// checkQuery validates a single VQL query and prints diagnostics. This is
// a convenient non-interactive way to test the validator, and can be used
// by agents as a pre-flight check.
func checkQuery(
	ctx context.Context, api_client api_proto.APIClient, query string) error {
	registry, err := buildRegistry(ctx, api_client)
	if err != nil {
		return err
	}

	for _, diag := range registry.Validate(query) {
		// Print diagnostics in a simple format:
		// line 2 col 5: message
		os.Stdout.WriteString(fmt.Sprintf("line %d col %d: %s\n",
			diag.Range.Start.Line+1, diag.Range.Start.Character+1,
			diagnosticMessage(diag)))
	}

	return nil
}

// stdioConn adapts the LSP stdio stream to an io.ReadWriteCloser.
type stdioConn struct {
	reader io.Reader
	writer io.Writer
}

func diagnosticMessage(diag protocol.Diagnostic) string {
	switch msg := diag.Message.(type) {
	case protocol.String:
		return string(msg)
	case *protocol.MarkupContent:
		return msg.Value
	default:
		return fmt.Sprint(diag.Message)
	}
}

func (self stdioConn) Read(p []byte) (int, error) {
	return self.reader.Read(p)
}

func (self stdioConn) Write(p []byte) (int, error) {
	return self.writer.Write(p)
}

func (self stdioConn) Close() error {
	return nil
}

// Keep kingpin imported even if flags change.
var _ = kingpin.CommandLine
