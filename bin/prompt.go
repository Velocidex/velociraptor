package main

import "fmt"

var (
	prompt_flag = app.Flag(
		"prompt", "Present a prompt before exit").Bool()
)

// Possibly ask for a prompt before exiting.
func doPrompt() {
	if *prompt_flag {
		fmt.Println("Press the Enter Key to end")
		fmt.Scanln()
	}
}
