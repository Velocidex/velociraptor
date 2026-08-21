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

	server := lsp_client.NewLSPProxy(api_client, log_file)
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

	stream := jsonrpc2.NewStream(stdio)
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