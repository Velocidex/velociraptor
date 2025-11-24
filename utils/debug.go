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
package utils

import (
	"context"
	"fmt"
	"os"
	"runtime/debug"
	"sync"
	"time"

	"github.com/davecgh/go-spew/spew"
)

func PrintStack() {
	debug.PrintStack()
}

func Debug(arg interface{}) {
	spew.Dump(arg)
}

func DlvBreak() {
	if false {
		fmt.Printf("Break")
		PrintStack()
	}
}

var (
	debugToFileMu sync.Mutex
)

func DebugToFile(filename, format string, v ...interface{}) {
	debugToFileMu.Lock()
	defer debugToFileMu.Unlock()

	fd, err := os.OpenFile(filename, os.O_RDWR|os.O_CREATE, 0700)
	if err != nil {
		panic(err)
	}
	defer fd.Close()

	_, _ = fd.Seek(0, os.SEEK_END)
	_, _ = fd.Write([]byte(fmt.Sprintf(format, v...) + "\n"))
}

type DebugStringer interface {
	DebugString() string
}

func DebugString(v interface{}) string {
	if IsNil(v) {
		return "nil"
	}

	switch t := v.(type) {
	case DebugStringer:
		return t.DebugString()

	default:
		return fmt.Sprintf("%T %v", v, v)
	}
}

// Check if a context is still valid
func DebugCtx(ctx context.Context, name string) {
	select {
	case <-ctx.Done():
		fmt.Printf("%s: Ctx is done!\n", name)

	default:
		fmt.Printf("%s: Ctx is still valid!\n", name)
	}
}

func DebugLogWhenCtxDone(ctx context.Context, name string) {
	go func() {
		<-ctx.Done()
		fmt.Printf("%s: Ctx done!\n", name)
	}()
}

func IsCtxDone(ctx context.Context) bool {
	select {
	case <-ctx.Done():
		return true
	default:
		return false
	}
}

// Alterantives to context.WithCancel to see when contexts are being
// cancelled.
func WithCancel(ctx context.Context) (context.Context, func()) {
	res, cancel := context.WithCancel(ctx)
	return res, func() {
		cancel()
	}
}

// Alterantives to context.WithTimeout to see when contexts are being
// cancelled.
func WithTimeout(ctx context.Context, wait time.Duration) (
	context.Context, func()) {
	res, cancel := context.WithTimeout(ctx, wait)
	return res, func() {
		DlvBreak()
		fmt.Printf("Timeout %v\n", wait)
		cancel()
	}
}
