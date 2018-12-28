// +build windows

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

	vql_subsystem "www.velocidex.com/golang/velociraptor/vql"
	"www.velocidex.com/golang/vfilter"
)

type ProcDumpArgs struct {
	Pid int64 `vfilter:"required,field=pid"`
}

type ProcDumpPlugin struct{}

func (self ProcDumpPlugin) Call(
	ctx context.Context,
	scope *vfilter.Scope,
	args *vfilter.Dict) <-chan vfilter.Row {
	output_chan := make(chan vfilter.Row)
	arg := &ProcDumpArgs{}

	go func() {
		defer close(output_chan)
		runtime.LockOSThread()
		defer runtime.UnlockOSThread()

		err := vfilter.ExtractArgs(scope, args, arg)
		if err != nil {
			scope.Log("proc_dump: %s", err.Error())
			return
		}

		tmpfile, err := ioutil.TempFile(os.TempDir(), "dmp.")
		if err != nil {
			scope.Log("proc_dump: %s", err.Error())
			return
		}

		// Close the file and remove it because the dump file
		// will be written in its place.
		filename := tmpfile.Name()
		tmpfile.Close()
		os.Remove(filename)

		scope.AddDesctructor(func() {
			os.Remove(filename)
		})

		c_filename := C.CString(filename)
		defer C.free(unsafe.Pointer(c_filename))

		res := C.dumpProcess(C.int(arg.Pid), c_filename)
		if int(res) == -1 {
			scope.Log("proc_dump: failed to dump process: %v", res)
			return
		}

		output_chan <- vfilter.NewDict().
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
