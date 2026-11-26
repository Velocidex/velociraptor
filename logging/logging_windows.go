// +build windows

package logging

import (
	"bytes"
	"fmt"
	"os"
	"strings"
	"syscall"
	"time"

	"github.com/rifflock/lfshook"
	"github.com/sirupsen/logrus"
	"golang.org/x/sys/windows"
	"www.velocidex.com/golang/velociraptor/json"
)

var (
	kernel32Dll    *windows.LazyDLL  = windows.NewLazySystemDLL("Kernel32.dll")
	setConsoleMode *windows.LazyProc = kernel32Dll.NewProc("SetConsoleMode")

	color_map = map[string]string{
		"reset":  "\033[0m",
		"red":    "\033[31m",
		"green":  "\033[32m",
		"yellow": "\033[33m",
		"blue":   "\033[34m",
		"purple": "\033[35m",
		"cyan":   "\033[36m",
		"gray":   "\033[37m",
		"white":  "\033[97m",
	}
)

func EnableVirtualTerminalProcessing(stream syscall.Handle, enable bool) error {
	const ENABLE_VIRTUAL_TERMINAL_PROCESSING uint32 = 0x4

	var mode uint32
	err := syscall.GetConsoleMode(stream, &mode)
	if err != nil {
		return err
	}

	if enable {
		mode |= ENABLE_VIRTUAL_TERMINAL_PROCESSING
	} else {
		mode &^= ENABLE_VIRTUAL_TERMINAL_PROCESSING
	}

	ret, _, err := setConsoleMode.Call(uintptr(stream), uintptr(mode))
	if ret == 0 {
		return err
	}

	return nil
}

type Formatter struct {
	stderr_map lfshook.WriterMap
}

func (self *Formatter) Format(entry *logrus.Entry) ([]byte, error) {
	b := &bytes.Buffer{}

	levelText := strings.ToUpper(entry.Level.String())
	fmt.Fprintf(b, "[%s] %v %s ", levelText, entry.Time.Format(time.RFC3339),
		strings.TrimRight(entry.Message, "\r\n"))

	if len(entry.Data) > 0 {
		serialized, _ := json.Marshal(entry.Data)
		fmt.Fprintf(b, "%s", serialized)
	}

	const enableVirtualTerminalProcessing uint32 = 0x4

	// Only print the result to the console, if there is an stderr
	// map to it.
	writer, pres := self.stderr_map[entry.Level]
	if pres && writer != nil {
		if NoColor {
			fmt.Fprintln(writer, clearTag(b.String()))
		} else {
			if f, ok := writer.(*os.File); ok {
				handle := syscall.Handle(f.Fd())
				var origMode uint32
				if syscall.GetConsoleMode(handle, &origMode) == nil &&
					origMode&enableVirtualTerminalProcessing == 0 {
					EnableVirtualTerminalProcessing(handle, true)
					defer EnableVirtualTerminalProcessing(handle, false)
				}
			}
			fmt.Fprintln(writer, replaceTagWithCode(b.String()))
		}
	}

	return nil, nil
}

func replaceTagWithCode(message string) string {
	if NoColor {
		return clearTag(message)
	}

	result := tag_regex.ReplaceAllStringFunc(message, func(hit string) string {
		matches := tag_regex.FindStringSubmatch(hit)
		if len(matches) > 1 {
			code, pres := color_map[matches[1]]
			if pres {
				return code
			}
		}
		return hit
	})

	reset := color_map["reset"]
	return closing_tag_regex.ReplaceAllString(result, reset) + reset
}
