package utils

import (
	"fmt"
	"runtime"
	"runtime/debug"

	"www.velocidex.com/golang/vfilter"
)

func CheckForPanic(msg string, vals ...interface{}) {
	r := recover()
	if r != nil {
		fmt.Printf(msg, vals...)
		fmt.Printf("PANIC %v\n", r)
		debug.PrintStack()
	}
}

func RecoverVQL(scope vfilter.Scope) {
	r := recover()
	if r != nil {
		scope.Log("PANIC: %v\n", r)
		buffer := make([]byte, 4096)
		n := runtime.Stack(buffer, false /* all */)
		scope.Log("%s", buffer[:n])
	}
}
