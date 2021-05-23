// +build windows,amd64

package authenticode

import (
	"fmt"
	"unsafe"

	windows "www.velocidex.com/golang/velociraptor/vql/windows"
)

func VerifyFileSignature(normalized_path string) string {
	err := windows.HasWintrustDll()
	if err != nil {
		return fmt.Sprintf("untrusted (%v)", err)
	}

	// Get filename in UTF16
	filename, err := windows.UTF16FromString(normalized_path)
	if err != nil {
		return fmt.Sprintf("untrusted (%v)", err)
	}

	// Make sure the filename is null terminated.
	filename = append(filename, 0)

	fi := new(windows.WINTRUST_FILE_INFO)
	fi.CbStruct = uint32(unsafe.Sizeof(*fi))
	fi.PcwszFilePath = (uintptr)(unsafe.Pointer(&filename[0]))

	trustData := new(windows.WINTRUST_DATA)
	trustData.CbStruct = uint32(unsafe.Sizeof(*trustData))
	trustData.DwUIChoice = windows.WTD_UI_NONE
	trustData.FdwRevocationChecks = windows.WTD_REVOKE_NONE
	trustData.DwUnionChoice = windows.WTD_CHOICE_FILE
	trustData.Union = (uintptr)(unsafe.Pointer(fi))
	trustData.DwStateAction = windows.WTD_STATEACTION_VERIFY
	trustData.DwProvFlags = windows.WTD_SAFER_FLAG

	ret, err := windows.WinVerifyTrust(
		windows.INVALID_HANDLE_VALUE,
		&WINTRUST_ACTION_GENERIC_VERIFY_V2,
		trustData)
	if err != nil {
		return fmt.Sprintf("untrusted (%v)", err)
	}

	// Any hWVTStateData must be released by a call with close.
	trustData.DwStateAction = windows.WTD_STATEACTION_CLOSE

	windows.WinVerifyTrust(windows.INVALID_HANDLE_VALUE,
		&DRIVER_ACTION_VERIFY, trustData)

	return WinVerifyTrustErrors(ret)
}
