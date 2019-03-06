package utils

import (
	"fmt"
	"runtime/debug"
)

func CheckForPanic(msg string, vals ...interface{}) {
	r := recover()
	if r != nil {
		fmt.Printf(msg, vals...)
		fmt.Printf("PANIC %v\n", r)
		debug.PrintStack()
	}
}
