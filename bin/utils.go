package main

import (
	"context"
	"os"
	"os/signal"

	vfilter "www.velocidex.com/golang/vfilter"
)

func InstallSignalHandler(
	scope *vfilter.Scope) context.Context {

	// Wait for signal. When signal is received we shut down the
	// server.
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, os.Interrupt)

	ctx, cancel := context.WithCancel(context.Background())
	scope.AddDesctructor(func() {
		cancel()
	})

	go func() {
		defer cancel()

		// Wait for the signal on this channel.
		<-quit
		scope.Log("Shutting down due to interrupt.")
		scope.Close()
	}()

	return ctx
}
