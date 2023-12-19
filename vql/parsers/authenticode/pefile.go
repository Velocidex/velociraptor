// +build windows,amd64

package authenticode

import (
	"fmt"
	"unsafe"

	"www.velocidex.com/golang/velociraptor/constants"
	vql_subsystem "www.velocidex.com/golang/velociraptor/vql"
	windows "www.velocidex.com/golang/velociraptor/vql/windows"
	"www.velocidex.com/golang/vfilter"
)

func VerifyFileSignature(
	scope vfilter.Scope,
	normalized_path string) string {

	if vql_subsystem.GetBoolFromRow(scope, scope,
		constants.DISABLE_DANGEROUS_API_CALLS) {
		return "Unknown"
	}

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
	fi.PcwszFilePath = &filename[0]

	trustData := new(windows.WINTRUST_DATA_FILE_INFO)
	trustData.CbStruct = uint32(unsafe.Sizeof(*trustData))
	trustData.DwUIChoice = windows.WTD_UI_NONE
	trustData.FdwRevocationChecks = windows.WTD_REVOKE_NONE
	trustData.DwUnionChoice = windows.WTD_CHOICE_FILE
	trustData.PFile = fi
	trustData.DwStateAction = windows.WTD_STATEACTION_VERIFY
	trustData.DwProvFlags = windows.WTD_SAFER_FLAG

	ret, err := windows.WinVerifyTrust(
		windows.INVALID_HANDLE_VALUE,
		&WINTRUST_ACTION_GENERIC_VERIFY_V2,
		trustData)

	// Any hWVTStateData must be released by a call with close.
	// Close the handle regardless of err above.
	trustData.DwStateAction = windows.WTD_STATEACTION_CLOSE

	windows.WinVerifyTrust(windows.INVALID_HANDLE_VALUE,
		&WINTRUST_ACTION_GENERIC_VERIFY_V2, trustData)

	if err != nil {
		return fmt.Sprintf("untrusted (%v)", err)
	}

	return WinVerifyTrustErrors(ret)
}
