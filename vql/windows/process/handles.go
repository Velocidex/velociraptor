//go:build windows && amd64 && cgo
// +build windows,amd64,cgo

// References: https://www.geoffchappell.com/studies/windows/km/ntoskrnl/api/ex/sysinfo/query.htm
// https://processhacker.sourceforge.io/

package process

import (
	"context"
	"errors"
	"fmt"
	"os"
	"runtime"
	"sync"
	"syscall"
	"time"
	"unsafe"

	"github.com/Velocidex/ordereddict"
	gowin "golang.org/x/sys/windows"
	"www.velocidex.com/golang/velociraptor/acls"
	"www.velocidex.com/golang/velociraptor/utils"
	"www.velocidex.com/golang/velociraptor/vql"
	vql_subsystem "www.velocidex.com/golang/velociraptor/vql"
	"www.velocidex.com/golang/velociraptor/vql/windows"
	"www.velocidex.com/golang/vfilter"
	"www.velocidex.com/golang/vfilter/arg_parser"
)

var (
	pool = sync.Pool{
		New: func() interface{} {
			buffer := utils.AllocateBuff(1024 * 10)
			return &buffer
		},
	}
)

type ThreadHandleInfo struct {
	ThreadId  uint64
	ProcessId uint64
	TokenInfo *TokenHandleInfo
}

type ProcessHandleInfo struct {
	Pid    uint32 `json:"Pid,omitempty"`
	Binary string `json:"Binary,omitempty"`
}

type TokenHandleInfo struct {
	IsElevated       bool     `json:"IsElevated"`
	User             string   `json:"User,omitempty"`
	Username         string   `json:"Username,omitempty"`
	ProfileDir       string   `json:"ProfileDir,omitempty"`
	Owner            string   `json:"Owner,omitempty"`
	PrimaryGroup     string   `json:"PrimaryGroup,omitempty"`
	PrimaryGroupName string   `json:"PrimaryGroupName,omitempty"`
	Groups           []string `json:"Groups,omitempty"`
}

type HandleInfo struct {
	Pid             uint32             `json:"Pid"`
	Type            string             `json:"Type"`
	Name            string             `json:"Name,omitempty"`
	Handle          uint32             `json:"Handle"`
	AccessMask      uint32             `json:"AccessMask"`
	AccessMaskPerms []string           `json:"AccessMaskPerms"`
	Attributes      uint32             `json:"Attributes"`
	ProcessInfo     *ProcessHandleInfo `json:"ProcessInfo,omitempty"`
	ThreadInfo      *ThreadHandleInfo  `json:"ThreadInfo,omitempty"`
	TokenInfo       *TokenHandleInfo   `json:"TokenInfo,omitempty"`
}

type HandlesPluginArgs struct {
	Pid   uint64   `vfilter:"optional,field=pid,doc=If specified only get handles from these PIDs."`
	Types []string `vfilter:"optional,field=types,doc=If specified only get handles of this type."`
}

type HandlesPlugin struct{}

func (self HandlesPlugin) Call(
	ctx context.Context,
	scope vfilter.Scope,
	args *ordereddict.Dict) <-chan vfilter.Row {
	output_chan := make(chan vfilter.Row)

	go func() {
		defer close(output_chan)
		defer vql_subsystem.RegisterMonitor(ctx, "handles", args)()

		err := vql_subsystem.CheckAccess(scope, acls.MACHINE_STATE)
		if err != nil {
			scope.Log("handles: %s", err)
			return
		}

		runtime.LockOSThread()

		// Deliberately do not unlock this thread - this will
		// cause Go to terminate it and start another one.
		// defer runtime.UnlockOSThread()

		defer vql_subsystem.CheckForPanic(scope, "handles")

		arg := &HandlesPluginArgs{}
		err = arg_parser.ExtractArgsWithContext(ctx, scope, args, arg)
		if err != nil {
			scope.Log("handles: %s", err.Error())
			return
		}

		err = TryToGrantSeDebugPrivilege()
		if err != nil {
			scope.Log("handles while trying to grant SeDebugPrivilege: %v", err)
		}

		GetHandles(ctx, scope, arg, output_chan)
	}()

	return output_chan
}

func (self HandlesPlugin) Info(scope vfilter.Scope, type_map *vfilter.TypeMap) *vfilter.PluginInfo {
	return &vfilter.PluginInfo{
		Name:     "handles",
		Doc:      "Enumerate process handles.",
		ArgType:  type_map.AddType(scope, &HandlesPluginArgs{}),
		Metadata: vql.VQLMetadata().Permissions(acls.MACHINE_STATE).Build(),
	}
}

func is_type_chosen(types []string, objtype string) bool {
	if len(types) == 0 {
		return true
	}

	for _, i := range types {
		if i == objtype {
			return true
		}
	}

	return false
}

// A sane version which allocates the right size buffer.
func SaneNtQuerySystemInformation(class uint32) ([]byte, error) {
	// Start off with something reasonable.
	buffer_size := 1024 * 1024 * 4
	var length uint32

	// A hard upper limit on the buffer.
	for buffer_size < 32*1024*1024 {
		buffer := utils.AllocateBuff(buffer_size)
		status := windows.NtQuerySystemInformation(class,
			&buffer[0], uint32(len(buffer)), &length)
		if status == windows.STATUS_SUCCESS {
			return buffer[:length], nil
		}

		// Buffer needs to grow
		if status == windows.STATUS_INFO_LENGTH_MISMATCH {
			buffer_size += 1024 * 1024 * 4
			continue
		}

		return nil, errors.New("NtQuerySystemInformation status " +
			windows.NTStatus_String(status))
	}
	return nil, errors.New("Too much memory needed")
}

func GetHandles(
	ctx context.Context,
	scope vfilter.Scope,
	arg *HandlesPluginArgs, out chan<- vfilter.Row) {
	// This should be large enough to fit all the handles.
	buffer, err := SaneNtQuerySystemInformation(windows.SystemHandleInformation)
	if err != nil {
		scope.Log("GetHandles: %v", err)
		return
	}

	// Group all handles by pid
	pid_map := make(map[int][]*windows.SYSTEM_HANDLE_TABLE_ENTRY_INFO64)

	// First pass, group all handles by pid.
	size := int(unsafe.Sizeof(windows.SYSTEM_HANDLE_TABLE_ENTRY_INFO64{}))
	for i := 8; i < len(buffer); i += size {
		handle_info := (*windows.SYSTEM_HANDLE_TABLE_ENTRY_INFO64)(unsafe.Pointer(
			uintptr(unsafe.Pointer(&buffer[0])) + uintptr(i)))

		pid := int(handle_info.UniqueProcessId)
		handle_group, _ := pid_map[pid]
		handle_group = append(handle_group, handle_info)
		pid_map[pid] = handle_group
	}

	// Now for each pid, inspect the handles carefully.
	for pid, handle_group := range pid_map {
		if arg.Pid != 0 && arg.Pid != uint64(pid) {
			continue
		}

		func() {
			process_handle := windows.NtCurrentProcess()
			my_pid := os.Getpid()

			// Open a handle to this process.
			if pid != my_pid {
				h, err := windows.OpenProcess(
					windows.PROCESS_DUP_HANDLE,
					false, uint32(pid))
				if err != nil {
					scope.Log("OpenProcess for pid %v: %v\n",
						GetProcessContext(ctx, scope, uint64(pid)), err)
					return
				}
				process_handle = h
				defer windows.CloseHandle(h)
			}

			// Duplicate each handle and query its details.
			for _, handle_info := range handle_group {
				handle_value := syscall.Handle(handle_info.HandleValue)

				// If we do not own the handle we need
				// to dup it into our process. If the
				// handle is already in our process we
				// can use it as is.
				if int(handle_info.UniqueProcessId) != my_pid {
					dup_handle := syscall.Handle(0)
					status := windows.NtDuplicateObject(
						process_handle, handle_value,
						windows.NtCurrentProcess(),
						&dup_handle,
						windows.PROCESS_QUERY_LIMITED_INFORMATION|
							syscall.TOKEN_QUERY|
							windows.THREAD_QUERY_LIMITED_INFORMATION, 0, 0)
					if status == windows.STATUS_SUCCESS {
						SendHandleInfo(
							arg, scope,
							handle_info,
							dup_handle, out)
						windows.CloseHandle(dup_handle)
					} else {
						// If we failed to dup the handle with extra
						// permissions try with no permissions - this
						// will give some information but better than
						// nothing.
						status := windows.NtDuplicateObject(
							process_handle, handle_value,
							windows.NtCurrentProcess(),
							&dup_handle, 0, 0, 0)
						if status == windows.STATUS_SUCCESS {
							SendHandleInfo(
								arg, scope,
								handle_info,
								dup_handle, out)
							windows.CloseHandle(dup_handle)
						}
					}
				} else {
					SendHandleInfo(
						arg, scope, handle_info,
						handle_value, out)
				}
			}
		}()
	}
}

func SendHandleInfo(arg *HandlesPluginArgs, scope vfilter.Scope,
	handle_info *windows.SYSTEM_HANDLE_TABLE_ENTRY_INFO64,
	handle syscall.Handle, out chan<- vfilter.Row) {

	to_send := false
	result := &HandleInfo{
		Pid:             uint32(handle_info.UniqueProcessId),
		Handle:          uint32(handle_info.HandleValue),
		AccessMask:      uint32(handle_info.GrantedAccess),
		AccessMaskPerms: parseMasks(handle_info.GrantedAccess, baseMasks),
		Attributes:      uint32(handle_info.HandleAttributes),
	}

	// Sometimes the NtQueryObject blocks without a
	// reason. Process Hacker uses a strategy where it launches
	// the call on another thread and actively kills the
	// thread. Instead we just sacrifice an Go thread. This may
	// not be ideal.
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	go func() {
		defer cancel()

		result.Type = GetObjectType(handle, scope)
		// Lazily skip handles we are not going to send anyway.
		if is_type_chosen(arg.Types, result.Type) {
			to_send = true
			switch result.Type {
			case "Process":
				result.ProcessInfo = GetProcessName(scope, handle)
				result.AccessMaskPerms = append(result.AccessMaskPerms,
					parseMasks(handle_info.GrantedAccess, processMasks)...)

			case "Thread":
				GetThreadInfo(scope, handle, result)
				result.AccessMaskPerms = append(result.AccessMaskPerms,
					parseMasks(handle_info.GrantedAccess, threadMasks)...)

			case "Event":
				result.AccessMaskPerms = append(result.AccessMaskPerms,
					parseMasks(handle_info.GrantedAccess, eventMasks)...)

			case "Token":
				result.TokenInfo = GetTokenInfo(scope, handle)
				result.AccessMaskPerms = append(result.AccessMaskPerms,
					parseMasks(handle_info.GrantedAccess, tokenMasks)...)

			case "Directory":
				result.AccessMaskPerms = append(result.AccessMaskPerms,
					parseMasks(handle_info.GrantedAccess, directoryMasks)...)

			case "Key":
				result.AccessMaskPerms = append(result.AccessMaskPerms,
					parseMasks(handle_info.GrantedAccess, keyMasks)...)

			case "Mutant":
				result.AccessMaskPerms = append(result.AccessMaskPerms,
					parseMasks(handle_info.GrantedAccess, mutantMasks)...)

			case "File":
				result.AccessMaskPerms = append(result.AccessMaskPerms,
					parseMasks(handle_info.GrantedAccess, fileMasks)...)
			}

			// Try to get the name if possible
			GetObjectName(scope, handle, result)
		}
	}()

	select {
	case <-ctx.Done():
		break
	}

	if to_send {
		out <- result
	}
}

func GetTokenInfo(scope vfilter.Scope, handle syscall.Handle) *TokenHandleInfo {
	token := gowin.Token(handle)
	result := &TokenHandleInfo{
		IsElevated: token.IsElevated(),
	}

	// Find the token user
	tokenUser, err := token.GetTokenUser()
	if err == nil &&
		tokenUser != nil &&
		tokenUser.User.Sid != nil {
		result.User = tokenUser.User.Sid.String()

		// look up domain account by sid
		result.Username = getUsernameFromSid(scope, tokenUser.User.Sid)
	}

	token_groups, err := token.GetTokenGroups()
	if err == nil {
		for _, grp := range token_groups.AllGroups() {
			group_name := grp.Sid.String()
			result.Groups = append(
				result.Groups, group_name)
		}
	}

	profile_dir, err := token.GetUserProfileDirectory()
	if err == nil {
		result.ProfileDir = profile_dir
	}
	pg, err := token.GetTokenPrimaryGroup()
	if err == nil && pg != nil && pg.PrimaryGroup != nil {
		result.PrimaryGroup = pg.PrimaryGroup.String()
		result.PrimaryGroupName = getUsernameFromSid(
			scope, pg.PrimaryGroup)
	}

	return result
}

func getUsernameFromSid(scope vfilter.Scope, sid *gowin.SID) string {
	key := sid.String()
	username_any := vql_subsystem.CacheGet(scope, key)
	if username_any != nil {
		return username_any.(string)
	}

	// Fetch the username from the API - if we fail the username is ""
	username := ""
	account, domain, _, err := sid.LookupAccount("localhost")
	if err == nil && account != "" {
		username = fmt.Sprintf("%s\\%s", domain, account)
	}
	vql_subsystem.CacheSet(scope, key, username)
	return username
}

func GetThreadInfo(scope vfilter.Scope, handle syscall.Handle, result *HandleInfo) {
	handle_info := windows.THREAD_BASIC_INFORMATION{}
	var length uint32

	status, _ := windows.NtQueryInformationThread(
		handle, windows.ThreadBasicInformation,
		(*byte)(unsafe.Pointer(&handle_info)),
		uint32(unsafe.Sizeof(handle_info)), &length)

	if status != windows.STATUS_SUCCESS {
		scope.Log("windows.NtQueryInformationProcess status %v", windows.NTStatus_String(status))
		return
	}

	result.ThreadInfo = &ThreadHandleInfo{
		ThreadId:  handle_info.UniqueThreadId,
		ProcessId: handle_info.UniqueProcessId,
	}

	// Try to get the token from the thread.
	token_handle := syscall.Handle(0)

	status = windows.NtOpenThreadToken(handle,
		syscall.TOKEN_READ, true, &token_handle)
	if status == windows.STATUS_SUCCESS {
		result.ThreadInfo.TokenInfo = GetTokenInfo(scope, token_handle)
		windows.CloseHandle(token_handle)

		// If the thread is not impersonating the error will be
		// STATUS_NO_TOKEN
	} else if status != windows.STATUS_NO_TOKEN &&
		status != windows.STATUS_ACCESS_DENIED {
		scope.Log("windows.NtOpenThreadToken status %v",
			windows.NTStatus_String(status))
	}
}

func GetProcessName(scope vfilter.Scope, handle syscall.Handle) *ProcessHandleInfo {
	buffer := utils.AllocateBuff(1024 * 2)

	handle_info := windows.PROCESS_BASIC_INFORMATION{}
	var length uint32

	status := windows.NtQueryInformationProcess(
		handle, windows.ProcessBasicInformation,
		(*byte)(unsafe.Pointer(&handle_info)),
		uint32(unsafe.Sizeof(handle_info)), &length)

	if status != windows.STATUS_SUCCESS {
		scope.Log("windows.NtQueryInformationProcess status %v", windows.NTStatus_String(status))
		return nil
	}

	result := &ProcessHandleInfo{Pid: handle_info.UniqueProcessId}

	// Fetch the binary image
	status = windows.NtQueryInformationProcess(
		handle, windows.ProcessImageFileName,
		(*byte)(unsafe.Pointer(&buffer[0])),
		uint32(len(buffer)), &length)

	if status != windows.STATUS_SUCCESS {
		return result
	}

	result.Binary = (*windows.UNICODE_STRING)(unsafe.Pointer(&buffer[0])).String()

	return result
}

func GetObjectName(scope vfilter.Scope, handle syscall.Handle, result *HandleInfo) {
	cached_buffer := pool.Get().(*[]byte)
	defer pool.Put(cached_buffer)

	buffer := *cached_buffer

	var length uint32

	status, _ := windows.NtQueryObject(handle, windows.ObjectNameInformation,
		&buffer[0], uint32(len(buffer)), &length)

	if status == windows.STATUS_INVALID_HANDLE {
		return
	}

	if status != windows.STATUS_SUCCESS {
		scope.Log("GetObjectName status %v", windows.NTStatus_String(status))

	} else {
		result.Name = (*windows.UNICODE_STRING)(unsafe.Pointer(&buffer[0])).String()
	}
}

func GetObjectType(handle syscall.Handle, scope vfilter.Scope) string {
	cached_buffer := pool.Get().(*[]byte)
	defer pool.Put(cached_buffer)

	buffer := *cached_buffer
	length := uint32(0)
	status, _ := windows.NtQueryObject(handle, windows.ObjectTypeInformation,
		&buffer[0], uint32(len(buffer)), &length)

	if status == windows.STATUS_SUCCESS {
		return (*windows.OBJECT_TYPE_INFORMATION)(
			unsafe.Pointer(&buffer[0])).TypeName.String()
	}
	scope.Log("GetObjectType status %v", windows.NTStatus_String(status))
	return ""
}

// Useful for access permissions.
func GetObjectBasicInformation(handle syscall.Handle) *windows.OBJECT_BASIC_INFORMATION {
	result := windows.OBJECT_BASIC_INFORMATION{}
	length := uint32(0)
	windows.NtQueryObject(handle, windows.ObjectBasicInformation,
		(*byte)(unsafe.Pointer(&result)), uint32(unsafe.Sizeof(result)), &length)

	return &result
}

func init() {
	vql_subsystem.RegisterPlugin(&HandlesPlugin{})
}

type mask struct {
	mask uint32
	name string
}

var (
	// https://learn.microsoft.com/en-us/windows/win32/procthread/process-security-and-access-rights
	baseMasks = []mask{
		{0x00010000, "DELETE"},
		{0x00020000, "READ_CONTROL"},
		{0x00040000, "WRITE_DAC"},
		{0x00080000, "WRITE_OWNER"},
		{0x00100000, "SYNCHRONIZE"},
	}

	// https://learn.microsoft.com/en-us/windows/win32/sync/synchronization-object-security-and-access-rights
	eventMasks = []mask{
		{0x000002, "EVENT_MODIFY_STATE"},
		{0x1F0003, "EVENT_ALL_ACCESS"},
	}

	// https://learn.microsoft.com/en-us/windows/win32/procthread/thread-security-and-access-rights
	threadMasks = []mask{
		{0x0001, "THREAD_TERMINATE"},
		{0x0002, "THREAD_SUSPEND_RESUME"},
		{0x0008, "THREAD_GET_CONTEXT"},
		{0x0010, "THREAD_SET_CONTEXT"},
		{0x0020, "THREAD_SET_INFORMATION"},
		{0x0040, "THREAD_QUERY_INFORMATION"},
		{0x0080, "THREAD_SET_THREAD_TOKEN"},
		{0x0100, "THREAD_IMPERSONATE"},
		{0x0200, "THREAD_DIRECT_IMPERSONATION"},
		{0x0400, "THREAD_SET_LIMITED_INFORMATION"},
		{0x0800, "THREAD_QUERY_LIMITED_INFORMATION"},
	}

	tokenMasks = []mask{
		{0x001, "TOKEN_ASSIGN_PRIMARY"},
		{0x002, "TOKEN_DUPLICATE"},
		{0x004, "TOKEN_IMPERSONATE"},
		{0x008, "TOKEN_QUERY"},
		{0x010, "TOKEN_QUERY_SOURCE"},
		{0x020, "TOKEN_ADJUST_PRIVILEGES"},
		{0x040, "TOKEN_ADJUST_GROUPS"},
		{0x080, "TOKEN_ADJUST_DEFAULT"},
		{0x100, "TOKEN_ADJUST_SESSIONID"},
	}

	processMasks = []mask{
		{0x0001, "PROCESS_TERMINATE"},
		{0x0002, "PROCESS_CREATE_THREAD"},
		{0x0008, "PROCESS_VM_OPERATION"},
		{0x0010, "PROCESS_VM_READ"},
		{0x0020, "PROCESS_VM_WRITE"},
		{0x0040, "PROCESS_DUP_HANDLE"},
		{0x0080, "PROCESS_CREATE_PROCESS"},
		{0x0100, "PROCESS_SET_QUOTA"},
		{0x0200, "PROCESS_SET_INFORMATION"},
		{0x0400, "PROCESS_QUERY_INFORMATION"},
		{0x0800, "PROCESS_SUSPEND_RESUME"},
		{0x1000, "PROCESS_QUERY_LIMITED_INFORMATION"},
	}

	directoryMasks = []mask{
		{0x0001, "QUERY"},
		{0x0002, "TRAVERSE"},
		{0x0004, "CREATE_OBJECT"},
		{0x0008, "CREATE_SUBDIRECTORY"},
	}

	mutantMasks = []mask{
		{0x0001, "MUTANT_QUERY_STATE"},
	}

	keyMasks = []mask{
		{0x0001, "QUERY_VALUE"},
		{0x0002, "SET_VALUE"},
		{0x0004, "CREATE_SUB_KEY"},
		{0x0008, "ENUMERATE_SUB_KEYS"},
		{0x0010, "NOTIFY"},
		{0x0020, "CREATE_LINK"},
		{0x0040, "WOW64_64KEY"},
		{0x0080, "WOW64_32KEY"},
	}

	fileMasks = []mask{
		{0x0001, "READ_DATA"},
		{0x0002, "ADD_FILE"},
		{0x0004, "APPEND_DATA"},
		{0x0008, "READ_EA"},
		{0x0010, "WRITE_EA"},
		{0x0020, "EXECUTE"},
		{0x0040, "DELETE_CHILD"},
		{0x0080, "READ_ATTRIBUTES"},
		{0x0100, "WRITE_ATTRIBUTES"},
	}
)

func parseMasks(m uint32, masks []mask) (res []string) {
	for _, mask := range masks {
		if m&mask.mask > 0 {
			res = append(res, mask.name)
		}
	}
	return res
}
