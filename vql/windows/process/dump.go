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

package process

// #cgo LDFLAGS: -ldbghelp
//
// #include <stdlib.h>
//
// int dumpProcess(int pid, char *filename);
import "C"

import (
	"context"
	"os"
	"runtime"
	"unsafe"

	"github.com/Velocidex/ordereddict"
	"www.velocidex.com/golang/velociraptor/accessors"
	"www.velocidex.com/golang/velociraptor/acls"
	"www.velocidex.com/golang/velociraptor/utils/tempfile"
	"www.velocidex.com/golang/velociraptor/vql"
	vql_subsystem "www.velocidex.com/golang/velociraptor/vql"
	"www.velocidex.com/golang/vfilter"
	"www.velocidex.com/golang/vfilter/arg_parser"
)

type ProcDumpPlugin struct{}

func (self ProcDumpPlugin) Call(
	ctx context.Context,
	scope vfilter.Scope,
	args *ordereddict.Dict) <-chan vfilter.Row {
	output_chan := make(chan vfilter.Row)
	arg := &PidArgs{}

	go func() {
		defer close(output_chan)
		defer vql_subsystem.RegisterMonitor(ctx, "proc_dump", args)()

		err := vql_subsystem.CheckAccess(scope, acls.MACHINE_STATE)
		if err != nil {
			scope.Log("proc_dump: %s", err)
			return
		}

		runtime.LockOSThread()
		defer runtime.UnlockOSThread()

		err = arg_parser.ExtractArgsWithContext(ctx, scope, args, arg)
		if err != nil {
			scope.Log("proc_dump: %s", err.Error())
			return
		}

		tmpfile, err := tempfile.TempFile("dmp")
		if err != nil {
			scope.Log("proc_dump: %s", err.Error())
			return
		}

		tempfile.AddTmpFile(tmpfile.Name())

		// Close the file and remove it because the dump file
		// will be written in its place.
		filename := tmpfile.Name()
		tmpfile.Close()

		err = os.Remove(filename)
		tempfile.RemoveTmpFile(filename, err)

		// Use a dmp extension to make it easier to open.
		filename += ".dmp"

		err = scope.AddDestructor(func() {
			os.Remove(filename)
		})
		if err != nil {
			os.Remove(filename)
			scope.Log("proc_dump: %v", err)
			return
		}

		c_filename := C.CString(filename)
		defer C.free(unsafe.Pointer(c_filename))

		res := C.dumpProcess(C.int(arg.Pid), c_filename)
		if int(res) == -1 {
			scope.Log("proc_dump: failed to dump process: %v", res)
			return
		}

		result := ordereddict.NewDict().
			Set("FullPath", filename).
			Set("Pid", arg.Pid)

		os_path, err := accessors.NewWindowsOSPath(filename)
		if err != nil {
			result.Set("OSPath", filename)
		} else {
			result.Set("OSPath", os_path)
		}

		select {
		case <-ctx.Done():
			return

		case output_chan <- result:
		}
	}()

	return output_chan
}

func (self ProcDumpPlugin) Info(scope vfilter.Scope, type_map *vfilter.TypeMap) *vfilter.PluginInfo {
	return &vfilter.PluginInfo{
		Name:     "proc_dump",
		Doc:      "Dumps process memory.",
		ArgType:  type_map.AddType(scope, &PidArgs{}),
		Metadata: vql.VQLMetadata().Permissions(acls.MACHINE_STATE).Build(),
	}
}

func init() {
	vql_subsystem.RegisterPlugin(&ProcDumpPlugin{})
}
