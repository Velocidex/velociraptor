//+build windows,amd64

package process

import (
	"syscall"
	"unsafe"

	"www.velocidex.com/golang/velociraptor/vql/windows"
)

type winLUID struct {
	LowPart  uint32
	HighPart uint32
}

// LUID_AND_ATTRIBUTES
type winLUIDAndAttributes struct {
	Luid       winLUID
	Attributes uint32
}

// TOKEN_PRIVILEGES
type winTokenPriviledges struct {
	PrivilegeCount uint32
	Privileges     [1]winLUIDAndAttributes
}

// Try to grant SeDebugPrivilege to outselves so we can inspect other
// processes.

func TryToGrantSeDebugPrivilege() error {
	// enable SeDebugPrivilege https://github.com/midstar/proci/blob/6ec79f57b90ba3d9efa2a7b16ef9c9369d4be875/proci_windows.go#L80-L119
	handle, err := syscall.GetCurrentProcess()
	if err != nil {
		return err
	}

	var token syscall.Token
	err = syscall.OpenProcessToken(handle, 0x0028, &token)
	if err != nil {
		return err
	}
	defer token.Close()

	tokenPriviledges := winTokenPriviledges{PrivilegeCount: 1}
	lpName := syscall.StringToUTF16("SeDebugPrivilege")
	err = windows.LookupPrivilegeValue(
		0, // Local System
		uintptr(unsafe.Pointer(&lpName[0])),
		uintptr(unsafe.Pointer(&tokenPriviledges.Privileges[0].Luid)))
	if err != nil {
		return err
	}

	tokenPriviledges.Privileges[0].Attributes = 0x00000002 // SE_PRIVILEGE_ENABLED

	err = windows.AdjustTokenPrivileges(
		token,
		false,
		uintptr(unsafe.Pointer(&tokenPriviledges)),
		int(unsafe.Sizeof(tokenPriviledges)),
		0,
		nil)
	if err != nil {
		return err
	}

	return nil
}
