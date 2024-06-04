//go:build windows

package efi

import (
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"syscall"
	"unsafe"

	"golang.org/x/sys/windows"
	"www.velocidex.com/golang/velociraptor/utils"
)

var (
	modntdll                                 = windows.NewLazySystemDLL("ntdll.dll")
	modkernel32                              = windows.NewLazySystemDLL("kernel32.dll")
	procNtEnumerateSystemEnvironmentValuesEx = modntdll.NewProc("NtEnumerateSystemEnvironmentValuesEx")
	procGetFirmwareEnvironmentVariableW      = modkernel32.NewProc("GetFirmwareEnvironmentVariableW")
)

func NtEnumerateSystemEnvironmentValuesEx(informationClass uint64, buffer *byte, bufferLength *uint64) error {
	r1, _, e1 := procNtEnumerateSystemEnvironmentValuesEx.Call(uintptr(informationClass), uintptr(unsafe.Pointer(buffer)), uintptr(unsafe.Pointer(bufferLength)))
	if r1 != 0 {
		return e1
	}
	return nil
}

func GetFirmwareEnvironmentVariable(name *uint16, guid *uint16, buffer *byte, size uint32) (uint32, error) {
	r1, _, e1 := procGetFirmwareEnvironmentVariableW.Call(uintptr(unsafe.Pointer(name)), uintptr(unsafe.Pointer(guid)), uintptr(unsafe.Pointer(buffer)), uintptr(size))
	if r1 == 0 {
		return 0, e1
	}
	return uint32(r1), nil
}

func GetEfiVariables() ([]EfiVariable, error) {
	var result []EfiVariable

	err := TryToGrantSeSystemEnvironmentPrivilege()
	if err != nil {
		return nil, err
	}

	bufferSize := uint64(1024 * 1024)
	buffer := make([]byte, bufferSize)

	err = NtEnumerateSystemEnvironmentValuesEx(1, &buffer[0], &bufferSize)
	if err != nil {
		return nil, err
	}

	reader := bytes.NewReader(buffer)
	currentOffset := uint32(0)
	var nextOffset uint32
	for {
		err = binary.Read(reader, binary.LittleEndian, &nextOffset)
		if err != nil {
			return nil, err
		}
		if nextOffset == 0 {
			break
		}
		elementSize := nextOffset - currentOffset
		var guid windows.GUID
		err = binary.Read(reader, binary.LittleEndian, &guid)
		if err != nil {
			return nil, err
		}
		nameString := make([]uint16, (elementSize-20)/2)
		err = binary.Read(reader, binary.LittleEndian, &nameString)
		if err != nil {
			return nil, err
		}

		result = append(result, EfiVariable{
			Namespace: fmt.Sprintf("{%08x-%04x-%04x-%04x-%012x}", guid.Data1, guid.Data2, guid.Data3, guid.Data4[:2], guid.Data4[2:]),
			Name:      windows.UTF16ToString(nameString),
			Value:     nil,
		})
	}

	return result, nil
}

func TryToGrantSeSystemEnvironmentPrivilege() error {
	p := windows.CurrentProcess()
	var token windows.Token
	err := windows.OpenProcessToken(p, windows.TOKEN_ADJUST_PRIVILEGES|windows.TOKEN_QUERY, &token)
	if err != nil {
		return err
	}

	defer token.Close()

	var luid windows.LUID
	err = windows.LookupPrivilegeValue(nil, windows.StringToUTF16Ptr("SeSystemEnvironmentPrivilege"), &luid)
	if err != nil {
		return err
	}

	ap := windows.Tokenprivileges{
		PrivilegeCount: 1,
	}
	ap.Privileges[0].Luid = luid
	ap.Privileges[0].Attributes = windows.SE_PRIVILEGE_ENABLED

	return windows.AdjustTokenPrivileges(token, false, &ap, 0, nil, nil)
}

func GetEfiVariableValue(namespace string, name string) ([]byte, error) {
	buff := utils.AllocateBuff(1024)
	length, err := GetFirmwareEnvironmentVariable(
		windows.StringToUTF16Ptr(name),
		windows.StringToUTF16Ptr(namespace),
		&buff[0], 1024)

	if errors.Is(err, syscall.ERROR_INSUFFICIENT_BUFFER) {
		buff = utils.AllocateBuff(32 * 1024)
		length, err = GetFirmwareEnvironmentVariable(
			windows.StringToUTF16Ptr(name),
			windows.StringToUTF16Ptr(namespace),
			&buff[0], 32*1024)
	}
	if err != nil {
		return nil, err
	}
	return buff[:length], nil
}
