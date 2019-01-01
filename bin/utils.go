package main

import (
	"os"
	"os/signal"
	"strings"

	vfilter "www.velocidex.com/golang/vfilter"
)

func hard_wrap(text string, colBreak int) string {
	text = strings.TrimSpace(text)
	wrapped := ""
	var i int
	for i = 0; len(text[i:]) > colBreak; i += colBreak {

		wrapped += text[i:i+colBreak] + "\n"

	}
	wrapped += text[i:]

	return wrapped
}

func InstallSignalHandler(
	scope *vfilter.Scope) {

	// Wait for signal. When signal is received we shut down the
	// server.
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, os.Interrupt)

	go func() {
		// Wait for the signal on this channel.
		<-quit
		scope.Log("Shutting down due to interrupt.")
		scope.Close()
	}()
}
