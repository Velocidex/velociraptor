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
	"sync"
	"time"

	"go.lsp.dev/jsonrpc2"
	"go.lsp.dev/protocol"
	"www.velocidex.com/golang/velociraptor/grpc_client"
	"www.velocidex.com/golang/velociraptor/logging"
	lsp_client "www.velocidex.com/golang/velociraptor/services/lsp/client"
)

var (
	lsp_cmd = app.Command("lsp",
		"Run a VQL language server on stdin/stdout (LSP over stdio).")

	lsp_log_file = lsp_cmd.Flag("log", "Write the LSP log to a file instead of stdout").
			String()

	lsp_log_verbose = lsp_cmd.Flag("log_verbose", "Increase LSP log verbosity "+
		"to include raw wire protocol dumps. Requires --log. Each "+
		"dump is delimited by ==== markers so pure protocol views "+
		"can be extracted mechanically. Payloads over 4kb are "+
		"truncated.").Bool()

	lsp_cmd_port = lsp_cmd.Flag(
		"port", "Is specified we listen on this TCP port, "+
			"otherwise use stdio").Int()
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

func listenOnTCP(
	ctx context.Context,
	server protocol.Server,
	port int) error {
	addr := fmt.Sprintf("127.0.0.1:%d", port)
	listener, err := net.Listen("tcp", addr)
	if err != nil {
		return err
	}
	defer listener.Close()

	go func() {
		for {
			netConn, err := listener.Accept()
			if err != nil {
				continue
			}

			fmt.Printf("LSP Server listening on TCP %s", addr)

			stream := jsonrpc2.NewStream(netConn)

			fmt.Printf("Starting server\n")

			server_ctx, conn, _ := protocol.NewServer(ctx, server, stream)

			// Block until the connection is closed by the client or the client
			// sends shutdown/exit (which closes server.Done()).
			select {
			case <-conn.Done():
				continue
			case <-server_ctx.Done():
				continue
			}
		}
	}()

	<-ctx.Done()
	return nil
}

func doLSP() error {
	logging.DisableLogging()

	config_obj, err := APIConfigLoader.WithNullLoader().
		LoadAndValidate()
	if err != nil {
		return err
	}

	ctx, cancel := Install_sig_handler()
	defer cancel()

	identity := grpc_client.SuperUser
	if config_obj.ApiConfig != nil && config_obj.ApiConfig.Name != "" {
		identity = grpc_client.API_User
	}

	// Make a remote query using the API - we better have user API
	// credentials in the config file.
	api_client, closer, err := grpc_client.Factory.GetAPIClient(
		ctx, identity, config_obj)
	if err != nil {
		return err
	}
	defer func() { _ = closer() }()

	var log_file *os.File
	if *lsp_log_file != "" {
		var err error
		log_file, err = os.OpenFile(*lsp_log_file,
			os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0600)
		if err != nil {
			return err
		}
		defer log_file.Close()
	}

	server := lsp_client.NewLSPProxy(
		ctx, api_client, identity, config_obj, log_file)
	// Listen on TCP
	if *lsp_cmd_port > 0 {
		return listenOnTCP(ctx, server, *lsp_cmd_port)
	}

	// The LSP protocol runs over stdio. It is important that nothing
	// else writes to stdout, so we may redirect logging elsewhere.
	stdio := stdioConn{
		reader: os.Stdin,
		writer: os.Stdout,
	}
	defer stdio.Close()

	// Optionally mirror all wire traffic into the log file.
	var stream_conn io.ReadWriteCloser = stdio
	if log_file != nil && *lsp_log_verbose {
		stream_conn = &loggingConn{
			ReadWriteCloser: stdio,
			log_file:        log_file,
		}
	}

	stream := jsonrpc2.NewStream(stream_conn)
	server_ctx, conn, _ := protocol.NewServer(ctx, server, stream)
	select {
	case <-conn.Done():
	case <-server_ctx.Done():
	case <-ctx.Done():
	}

	return nil
}

// stdioConn adapts the LSP stdio stream to an io.ReadWriteCloser.
type stdioConn struct {
	reader io.Reader
	writer io.Writer
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

// maxWireLogPayload caps how much of each frame is written to the
// wire log. Document syncs carry the full document text on every
// keystroke so unbounded dumps would flood the log with content we
// already have in the event log and in the editor itself.
const maxWireLogPayload = 4096

// loggingConn wraps the stdio stream and mirrors all traffic
// crossing it into the log file. This provides visibility into the
// raw protocol which is essential when debugging framing or
// ordering issues that structured event logs can not show. Reads
// are marked <---- (from the editor), writes ----> (to the editor).
//
// Each dump is wrapped in ==== markers so tooling can extract a
// pure protocol view with e.g.
//
//	awk '/^==== wire/,/^==== end/' logfile
//
// The header always shows shown/total bytes so truncation is
// visible to analyzers even when the payload itself is capped.
type loggingConn struct {
	io.ReadWriteCloser
	log_file *os.File
	mu       sync.Mutex
}

func (self *loggingConn) dump(direction string, p []byte) {
	self.mu.Lock()
	defer self.mu.Unlock()

	shown := len(p)
	if shown > maxWireLogPayload {
		shown = maxWireLogPayload
	}

	fmt.Fprintf(self.log_file, "\n==== wire %v %v (%d/%d bytes) ====\n",
		direction, time.Now().Format(time.RFC3339Nano),
		shown, len(p))
	self.log_file.Write(p[:shown])
	// Payloads do not necessarily end with a newline so add one to
	// keep the end marker on its own line. Parsers should treat the
	// section contents as payload plus exactly one newline.
	fmt.Fprintf(self.log_file, "\n==== end ====\n")
}

func (self *loggingConn) Read(p []byte) (int, error) {
	n, err := self.ReadWriteCloser.Read(p)
	if n > 0 {
		self.dump("<----", p[:n])
	}
	return n, err
}

func (self *loggingConn) Write(p []byte) (int, error) {
	self.dump("---->", p)
	return self.ReadWriteCloser.Write(p)
}
