package main

import (
	"fmt"

	"github.com/inconshreveable/mousetrap"
)

var (
	prompt_flag = app.Flag(
		"prompt", "Present a prompt before exit").Bool()
)

// Possibly ask for a prompt before exiting.
func doPrompt() {
	if *prompt_flag || mousetrap.StartedByExplorer() {
		fmt.Println("Press the Enter Key to end")
		_, _ = fmt.Scanln()
	}
}
