// +build windows

/*
   Velociraptor - Hunting Evil
   Copyright (C) 2019 Velocidex Innovations.

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
	"io/ioutil"
	"os"
	"runtime"
	"unsafe"

	"github.com/Velocidex/ordereddict"
	"www.velocidex.com/golang/velociraptor/acls"
	vql_subsystem "www.velocidex.com/golang/velociraptor/vql"
	"www.velocidex.com/golang/vfilter"
)

type ProcDumpPlugin struct{}

func (self ProcDumpPlugin) Call(
	ctx context.Context,
	scope *vfilter.Scope,
	args *ordereddict.Dict) <-chan vfilter.Row {
	output_chan := make(chan vfilter.Row)
	arg := &ProcDumpArgs{}

	go func() {
		defer close(output_chan)

		err := vql_subsystem.CheckAccess(scope, acls.MACHINE_STATE)
		if err != nil {
			scope.Log("proc_dump: %s", err)
			return
		}

		runtime.LockOSThread()
		defer runtime.UnlockOSThread()

		err = vfilter.ExtractArgs(scope, args, arg)
		if err != nil {
			scope.Log("proc_dump: %s", err.Error())
			return
		}

		tmpfile, err := ioutil.TempFile(os.TempDir(), "dmp")
		if err != nil {
			scope.Log("proc_dump: %s", err.Error())
			return
		}

		// Close the file and remove it because the dump file
		// will be written in its place.
		filename := tmpfile.Name()
		tmpfile.Close()
		os.Remove(filename)

		// Use a dmp extension to make it easier to open.
		filename += ".dmp"

		scope.AddDestructor(func() {
			os.Remove(filename)
		})

		c_filename := C.CString(filename)
		defer C.free(unsafe.Pointer(c_filename))

		res := C.dumpProcess(C.int(arg.Pid), c_filename)
		if int(res) == -1 {
			scope.Log("proc_dump: failed to dump process: %v", res)
			return
		}

		output_chan <- ordereddict.NewDict().
			Set("FullPath", filename).
			Set("Pid", arg.Pid)
	}()

	return output_chan
}

func (self ProcDumpPlugin) Info(scope *vfilter.Scope, type_map *vfilter.TypeMap) *vfilter.PluginInfo {
	return &vfilter.PluginInfo{
		Name:    "proc_dump",
		Doc:     "Dumps process memory.",
		ArgType: type_map.AddType(scope, &ProcDumpArgs{}),
	}
}

func init() {
	vql_subsystem.RegisterPlugin(&ProcDumpPlugin{})
}
