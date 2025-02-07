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
	"io"
	"os"
	"os/signal"
	"syscall"

	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	logging "www.velocidex.com/golang/velociraptor/logging"
	"www.velocidex.com/golang/velociraptor/utils"
	vfilter "www.velocidex.com/golang/vfilter"
)

func InstallSignalHandler(
	top_ctx context.Context, scope vfilter.Scope) (context.Context, func()) {

	// Wait for signal. When signal is received we shut down the
	// server.
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGHUP,
		syscall.SIGINT,
		syscall.SIGTERM,
		syscall.SIGQUIT)

	if top_ctx == nil {
		top_ctx = context.Background()
	}

	subctx, cancel := context.WithCancel(top_ctx)
	go func() {
		select {
		// Wait for the signal on this channel.
		case <-quit:
			scope.Log("Shutting down due to interrupt.")

			scope.Close()
			// Only cancel the context once the scope is fully
			// destroyed. This ensures all the destructors have
			// enougb time to finish when we exit the program
			cancel()
		case <-subctx.Done():
		}
	}()

	return subctx, cancel
}

// Turns os.Stdout into into file_store.WriteSeekCloser
type StdoutWrapper struct {
	io.Writer
}

func (self *StdoutWrapper) Seek(offset int64, whence int) (int64, error) {
	return 0, nil
}

func (self *StdoutWrapper) Close() error {
	return nil
}

func (self *StdoutWrapper) Truncate(offset int64) error {
	return nil
}

// This is only called when there is something very wrong! If the
// executor loop somehow exits due to panic or a bug we will not be
// able to communicate with the endpoint. We have to hard exit here to
// ensure the process can be restarted. This is a last resort!
func on_error(ctx context.Context, config_obj *config_proto.Config) {
	select {

	// It's ok we are supposed to exit.
	case <-ctx.Done():
		return

	default:
		// Log the error.
		logger := logging.GetLogger(config_obj, &logging.ClientComponent)
		logger.Error("Exiting hard due to bug or KillKillKill! This should not happen!")
		r := recover()
		if r != nil {
			utils.Debug(r)
		}
		utils.PrintStack()

		os.Exit(-1)
	}
}

func install_sig_handler() (context.Context, context.CancelFunc) {
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGHUP,
		syscall.SIGINT,
		syscall.SIGTERM,
		syscall.SIGQUIT)

	ctx, cancel := context.WithCancel(context.Background())

	go func() {
		select {
		case <-quit:
			// Ordered shutdown now.
			cancel()

		case <-ctx.Done():
			return
		}
	}()

	return ctx, cancel
}

func isConfigSpecified(argv []string) bool {
	for _, a := range argv {
		if a == "--config" {
			return true
		}
	}
	return false
}
