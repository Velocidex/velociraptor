//go:build windows && amd64
// +build windows,amd64

// References
// http://blogs.microsoft.co.il/pavely/2014/02/05/creating-a-winobj-like-tool/

package process

import (
	"context"
	"path/filepath"
	"runtime"
	"unsafe"

	"github.com/Velocidex/ordereddict"
	"github.com/hillu/go-ntdll"
	"www.velocidex.com/golang/velociraptor/acls"
	"www.velocidex.com/golang/velociraptor/utils"
	"www.velocidex.com/golang/velociraptor/vql"
	vql_subsystem "www.velocidex.com/golang/velociraptor/vql"
	"www.velocidex.com/golang/velociraptor/vql/windows"
	"www.velocidex.com/golang/vfilter"
	"www.velocidex.com/golang/vfilter/arg_parser"
)

type WinObjDesc struct {
	Name          string
	Type          string
	SymlinkTarget string `json:"SymlinkTarget,omitempty"`
}

type WinObjPluginArgs struct {
	Path string `vfilter:"optional,field=path,doc=Object namespace path."`
}

type WinObjPlugin struct{}

func (self WinObjPlugin) Call(
	ctx context.Context,
	scope vfilter.Scope,
	args *ordereddict.Dict) <-chan vfilter.Row {
	output_chan := make(chan vfilter.Row)

	go func() {
		defer close(output_chan)
		defer vql_subsystem.RegisterMonitor(ctx, "winobj", args)()

		err := vql_subsystem.CheckAccess(scope, acls.MACHINE_STATE)
		if err != nil {
			scope.Log("winobj: %s", err)
			return
		}

		runtime.LockOSThread()

		// Deliberately do not unlock this thread - this will
		// cause Go to terminate it and start another one.
		// defer runtime.UnlockOSThread()

		defer vql_subsystem.CheckForPanic(scope, "winobj")

		arg := &WinObjPluginArgs{}
		err = arg_parser.ExtractArgsWithContext(ctx, scope, args, arg)
		if err != nil {
			scope.Log("winobj: %s", err.Error())
			return
		}

		path := filepath.Clean("\\" + arg.Path)
		GetObjects(ctx, scope, path, output_chan, 0)
	}()

	return output_chan
}

func (self WinObjPlugin) Info(scope vfilter.Scope, type_map *vfilter.TypeMap) *vfilter.PluginInfo {
	return &vfilter.PluginInfo{
		Name:     "winobj",
		Doc:      "Enumerate The Windows Object Manager namespace.",
		ArgType:  type_map.AddType(scope, &WinObjPluginArgs{}),
		Metadata: vql.VQLMetadata().Permissions(acls.MACHINE_STATE).Build(),
	}
}

// GetObjects recursively traverses the object manager directories.
func GetObjects(ctx context.Context,
	scope vfilter.Scope,
	path string,
	output_chan chan<- vfilter.Row, depth int) {

	select {
	case <-ctx.Done():
		return
	default:
		if depth > 5 {
			return
		}
	}

	obj_attr := ntdll.NewObjectAttributes(path, 0, 0, nil)
	dir_handle := ntdll.Handle(0)

	status := ntdll.NtOpenDirectoryObject(&dir_handle,
		ntdll.DIRECTORY_QUERY|ntdll.DIRECTORY_TRAVERSE, obj_attr)
	if status != ntdll.STATUS_SUCCESS {
		scope.Log("winobj: %v for %v", status, path)
		return
	}
	defer ntdll.NtClose(dir_handle)

	buffer := utils.AllocateBuff(1024 * 1024)
	length := uint32(0)
	index := uint32(0)

	status = ntdll.NtQueryDirectoryObject(
		dir_handle, &buffer[0], uint32(len(buffer)),
		false, true, &index, &length)
	if status == ntdll.STATUS_NO_MORE_ENTRIES {
		return
	}

	if status != ntdll.STATUS_SUCCESS {
		scope.Log("winobj: %v for %v", status, path)
		return
	}

	object_directory_infos := []*ntdll.ObjectDirectoryInformationT{}
	size_of_info := uint32(unsafe.Sizeof(ntdll.ObjectDirectoryInformationT{}))
	for i := uint32(0); i < index; i++ {
		item := (*ntdll.ObjectDirectoryInformationT)(unsafe.Pointer(&buffer[i*size_of_info]))
		if item == nil {
			continue
		}

		object_directory_infos = append(object_directory_infos, item)

		info := &WinObjDesc{
			Name: filepath.Join(path, item.Name.String()),
			Type: item.TypeName.String(),
		}
		descObject(scope, info)
		select {
		case <-ctx.Done():
			return
		case output_chan <- info:
		}

		if item.TypeName.String() == "Directory" {
			GetObjects(ctx, scope,
				filepath.Join(path, item.Name.String()),
				output_chan, depth+1)
		}
	}
}

// Encrich the WinObjDesc with additional information
func descObject(scope vfilter.Scope, info *WinObjDesc) {
	switch info.Type {
	case "SymbolicLink":
		obj_attr := ntdll.NewObjectAttributes(info.Name, 0, 0, nil)
		handle := ntdll.Handle(0)
		status := ntdll.NtOpenSymbolicLinkObject(
			&handle, windows.SYMBOLIC_LINK_QUERY, obj_attr)
		if status == ntdll.STATUS_SUCCESS {
			defer ntdll.NtClose(handle)

			length := uint32(1024)
			symlink := ntdll.NewEmptyUnicodeString(1024)
			status := ntdll.NtQuerySymbolicLinkObject(
				handle, symlink, &length)
			if status == ntdll.STATUS_SUCCESS {
				info.SymlinkTarget = symlink.String()
			}
		}

	}
}

func init() {
	vql_subsystem.RegisterPlugin(&WinObjPlugin{})
}
