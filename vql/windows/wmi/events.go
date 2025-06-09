//go:build windows && cgo
// +build windows,cgo

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

// These VQL plugins deal with Windows WMI.
package wmi

// #cgo LDFLAGS: -lole32 -lwbemuuid -loleaut32 -luuid
//
// #include <stdlib.h>
//
// void *watchEvents(void *go_ctx, char *query, char *namespace);
//
// void destroyEvent(void *c_ctx);
import "C"

import (
	"context"
	"runtime"
	"time"
	"unsafe"

	"github.com/Velocidex/ordereddict"
	ole "github.com/go-ole/go-ole"
	pointer "github.com/mattn/go-pointer"
	"www.velocidex.com/golang/velociraptor/acls"
	"www.velocidex.com/golang/velociraptor/vql"
	vql_subsystem "www.velocidex.com/golang/velociraptor/vql"
	wmi_parse "www.velocidex.com/golang/velociraptor/vql/windows/wmi/parse"
	vfilter "www.velocidex.com/golang/vfilter"
	"www.velocidex.com/golang/vfilter/arg_parser"
)

type WMIObject struct {
	Raw    string
	parsed *ordereddict.Dict
}

func (self *WMIObject) Parse() (*ordereddict.Dict, error) {
	if self.parsed != nil {
		return self.parsed, nil
	}

	mof, err := wmi_parse.Parse(self.Raw)
	if err != nil {
		return ordereddict.NewDict(), err
	}
	self.parsed = mof.ToDict()
	return self.parsed, nil
}

type eventQueryContext struct {
	output chan vfilter.Row
	scope  vfilter.Scope
}

// This is called to handle the serialized event string. We just send
// it down the channel.
func (self *eventQueryContext) ProcessEvent(event string) {
	select {
	case self.output <- &WMIObject{Raw: event}:
	default:
		// We can not send the message because the queue is
		// too full. We have no choice but to drop it.
	}
}

func (self *eventQueryContext) Log(message string) {
	self.scope.Log(message)
}

//export process_event
func process_event(ctx *C.int, bstring **C.ushort) {
	go_ctx, ok := pointer.Restore(unsafe.Pointer(ctx)).(*eventQueryContext)
	if ok {
		text := ole.BstrToString(*(**uint16)(unsafe.Pointer(bstring)))
		go_ctx.ProcessEvent(text)
	}
}

//export log_error
func log_error(ctx *C.int, message *C.char) {
	go_ctx := pointer.Restore(unsafe.Pointer(ctx)).(*eventQueryContext)
	go_ctx.Log(C.GoString(message))
}

type WmiEventPluginArgs struct {
	Query     string `vfilter:"required,field=query,doc=WMI query to run."`
	Namespace string `vfilter:"required,field=namespace,doc=WMI namespace"`

	// How long to wait for events.
	Wait int64 `vfilter:"required,field=wait,doc=Wait this many seconds for events and then quit."`
}

type WmiEventPlugin struct{}

func (self WmiEventPlugin) Call(
	ctx context.Context,
	scope vfilter.Scope,
	args *ordereddict.Dict) <-chan vfilter.Row {
	output_chan := make(chan vfilter.Row)
	arg := &WmiEventPluginArgs{}

	go func() {
		defer close(output_chan)
		defer vql_subsystem.RegisterMonitor(ctx, "wmi_events", args)()

		err := vql_subsystem.CheckAccess(scope, acls.MACHINE_STATE)
		if err != nil {
			scope.Log("wmi_events: %s", err)
			return
		}

		runtime.LockOSThread()
		defer runtime.UnlockOSThread()

		err = arg_parser.ExtractArgsWithContext(ctx, scope, args, arg)
		if err != nil {
			scope.Log("wmi_events: %s", err.Error())
			return
		}

		if arg.Namespace == "" {
			arg.Namespace = "ROOT/CIMV2"
		}

		sub_ctx, cancel := context.WithTimeout(
			ctx, time.Duration(arg.Wait)*time.Second)
		defer cancel()

		event_context := eventQueryContext{
			// Queue up to 100 messages
			output: make(chan vfilter.Row, 100),
			scope:  scope,
		}
		defer close(event_context.output)

		ptr := pointer.Save(&event_context)
		defer pointer.Unref(ptr)

		c_query := C.CString(arg.Query)
		defer C.free(unsafe.Pointer(c_query))

		c_nsp := C.CString(arg.Namespace)
		defer C.free(unsafe.Pointer(c_nsp))

		c_ctx := C.watchEvents(ptr, c_query, c_nsp)
		if c_ctx == nil {
			return
		}

		for item := range event_context.output {
			select {
			case <-sub_ctx.Done():
				// Destroy the C context - we are done here.
				C.destroyEvent(c_ctx)
				return

				// Read the next item from the event
				// queue and send it to the VQL
				// subsystem.
			case output_chan <- item:
			}
		}
	}()

	return output_chan
}

func (self WmiEventPlugin) Info(scope vfilter.Scope, type_map *vfilter.TypeMap) *vfilter.PluginInfo {
	return &vfilter.PluginInfo{
		Name:     "wmi_events",
		Doc:      "Executes an evented WMI queries asynchronously.",
		ArgType:  type_map.AddType(scope, &WmiEventPluginArgs{}),
		Metadata: vql.VQLMetadata().Permissions(acls.MACHINE_STATE).Build(),
	}
}

func init() {
	vql_subsystem.RegisterPlugin(&WmiEventPlugin{})
}
