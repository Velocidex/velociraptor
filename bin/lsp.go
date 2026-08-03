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
	"os"

	"github.com/alecthomas/kingpin/v2"
	"go.lsp.dev/jsonrpc2"
	"go.lsp.dev/protocol"
	"www.velocidex.com/golang/velociraptor/logging"
	vql_subsystem "www.velocidex.com/golang/velociraptor/vql"
	"www.velocidex.com/golang/velociraptor/vql/lsp"
)

var (
	lsp_cmd = app.Command("lsp",
		"Run a VQL language server on stdin/stdout (LSP over stdio).")

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

	// If we are asked to check a query, do it and exit. This is useful
	// for testing and for agents that just want validation.
	if *lsp_check_plugin != "" {
		return checkQuery(*lsp_check_plugin)
	}

	scope := vql_subsystem.MakeScope()
	defer scope.Close()

	registry := lsp.BuildRegistry(scope)

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

// checkQuery validates a single VQL query and prints diagnostics. This is
// a convenient non-interactive way to test the validator, and can be used
// by agents as a pre-flight check.
func checkQuery(query string) error {
	scope := vql_subsystem.MakeScope()
	defer scope.Close()

	registry := lsp.BuildRegistry(scope)

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
