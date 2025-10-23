//go:build windows && amd64
// +build windows,amd64

package authenticode

import (
	"errors"
	"fmt"
	"os"
	"syscall"
	"unsafe"

	"github.com/Velocidex/ordereddict"
	"github.com/Velocidex/pkcs7"
	"www.velocidex.com/golang/go-pe"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	"www.velocidex.com/golang/velociraptor/constants"
	"www.velocidex.com/golang/velociraptor/utils"
	vql_subsystem "www.velocidex.com/golang/velociraptor/vql"
	windows "www.velocidex.com/golang/velociraptor/vql/windows"
	"www.velocidex.com/golang/vfilter"
)

const (
	TRUST_E_PROVIDER_UNKNOWN     = 0x800B0001
	TRUST_E_ACTION_UNKNOWN       = 0x800B0002
	TRUST_E_SUBJECT_FORM_UNKNOWN = 0x800B0003
	TRUST_E_SUBJECT_NOT_TRUSTED  = 0x800B0004
)

var (
	WINTRUST_ACTION_GENERIC_VERIFY_V2 = windows.GUID{0xaac56b, 0xcd44, 0x11d0,
		[8]byte{0x8c, 0xc2, 0x0, 0xc0, 0x4f, 0xc2, 0x95, 0xee}}

	DRIVER_ACTION_VERIFY = windows.GUID{0xf750e6c3, 0x38ee, 0x11d1,
		[8]byte{0x85, 0xe5, 0x0, 0xc0, 0x4f, 0xc2, 0x95, 0xee}}
)

func VerifyCatalogSignature(
	config_obj *config_proto.Config,
	scope vfilter.Scope,
	fd *os.File, normalized_path string,
	output *ordereddict.Dict) (string, error) {

	if vql_subsystem.GetBoolFromRow(scope, scope,
		constants.DISABLE_DANGEROUS_API_CALLS) {
		output.Update("Trusted", "Unknown")
		return "", nil
	}

	err := windows.HasWintrustDll()
	if err != nil {
		return "", err
	}

	// Get filename in UTF16
	filename, err := windows.UTF16FromString(normalized_path)
	if err != nil {
		return "", err
	}

	var CatAdminHandle syscall.Handle
	err = windows.CryptCATAdminAcquireContext2(&CatAdminHandle, nil, nil, nil, 0)
	if err != nil {
		return "", err
	}
	defer windows.CryptCATAdminReleaseContext(CatAdminHandle, 0)

	hash_length := uint32(100)
	hash := utils.AllocateBuff(100)

	err = windows.CryptCATAdminCalcHashFromFileHandle2(CatAdminHandle, fd.Fd(),
		&hash_length, &hash[0], 0)
	if err != nil {
		return "", err
	}

	// Find the first catalog containing this hash
	var CatInfoHandle syscall.Handle
	CatInfoHandle = windows.CryptCATAdminEnumCatalogFromHash(
		CatAdminHandle,
		&hash[0],
		hash_length,
		0,
		nil)
	if CatAdminHandle == 0 {
		return "", errors.New("Unable to get cat file.")
	}
	defer windows.CryptCATAdminReleaseCatalogContext(CatAdminHandle, CatInfoHandle, 0)

	// Cat file not found
	if CatInfoHandle == 0 {
		return "", nil
	}

	var catalogInfo windows.CATALOG_INFO

	catalogInfo.CbStruct = uint32(unsafe.Sizeof(catalogInfo))
	err = windows.CryptCATCatalogInfoFromContext(
		CatInfoHandle,
		&catalogInfo,
		0)
	if err != nil {
		output.Update("Trusted", fmt.Sprintf("untrusted (Error: %v)", err))
		return "", err
	}

	cat_file := windows.UTF16ToString(catalogInfo.WszCatalogFile[:])

	// Calculate the member tag - it is usually the hex
	// string of the hash but not always.
	tag := fmt.Sprintf("%X\x00", hash[:hash_length])
	tag_bytes, _ := windows.UTF16PtrFromString(tag)

	// Now figure out the signer from the cat file.
	ci := new(windows.WINTRUST_CATALOG_INFO)
	ci.CbStruct = uint32(unsafe.Sizeof(*ci))
	ci.PcwszCatalogFilePath = &catalogInfo.WszCatalogFile[0]
	ci.PcwszMemberFilePath = &filename[0]
	ci.HMemberFile = syscall.Handle(fd.Fd())
	ci.PcwszMemberTag = tag_bytes
	ci.PbCalculatedFileHash = &hash[0]
	ci.CbCalculatedFileHash = hash_length
	ci.HCatAdmin = CatAdminHandle

	trustData := new(windows.WINTRUST_DATA_CATALOG_INFO)
	trustData.CbStruct = uint32(unsafe.Sizeof(*trustData))
	trustData.DwUIChoice = windows.WTD_UI_NONE
	trustData.FdwRevocationChecks = windows.WTD_REVOKE_NONE
	trustData.DwUnionChoice = windows.WTD_CHOICE_CATALOG
	trustData.PCatalog = ci
	trustData.DwStateAction = windows.WTD_STATEACTION_VERIFY
	trustData.DwProvFlags = windows.WTD_SAFER_FLAG

	ret, _ := windows.WinVerifyTrust(windows.INVALID_HANDLE_VALUE,
		&WINTRUST_ACTION_GENERIC_VERIFY_V2, trustData)
	output.Update("Trusted", WinVerifyTrustErrors(ret))

	// Any hWVTStateData must be released by a call with close.
	trustData.DwStateAction = windows.WTD_STATEACTION_CLOSE

	windows.WinVerifyTrust(windows.INVALID_HANDLE_VALUE,
		&WINTRUST_ACTION_GENERIC_VERIFY_V2, trustData)

	return cat_file, nil
}

func ParseCatFile(cat_file string, output *ordereddict.Dict, verbose bool) error {

	// Set the catalog file even if we can not read it - will be
	// replaced later with the proper parse.
	output.Update("_ExtraInfo", ordereddict.NewDict().
		Set("Catalog", cat_file))

	cat_fd, err := os.Open(cat_file)
	if err != nil {
		return err
	}
	data, err := utils.ReadAllWithLimit(cat_fd, constants.MAX_MEMORY)
	if err != nil {
		return err
	}

	pkcs7_obj, err := pkcs7.Parse([]byte(data))
	if err != nil {
		return err
	}

	signer := pe.PKCS7ToOrderedDict(pkcs7_obj)
	if verbose {
		output.Update("_ExtraInfo", signer.Set("Catalog", cat_file))
	}

	output.Update("SubjectName", utils.GetString(signer, "Signer.Subject")).
		Update("IssuerName", utils.GetString(signer, "Signer.IssuerName")).
		Update("SerialNumber", utils.GetString(signer, "Signer.SerialNumber")).
		Update("ProgramName", utils.GetString(signer, "Signer.AuthenticatedAttributes.ProgramName")).
		Update("MoreInfoLink", utils.GetString(signer, "Signer.AuthenticatedAttributes.MoreInfo")).
		Update("Timestamp", utils.GetAny(signer, "Signer.AuthenticatedAttributes.SigningTime"))

	return nil
}

func WinVerifyTrustErrors(ret uint32) string {
	switch ret {
	case 0:
		return "trusted"
	case TRUST_E_SUBJECT_NOT_TRUSTED:
		return "TRUST_E_SUBJECT_NOT_TRUSTED"
	case TRUST_E_SUBJECT_FORM_UNKNOWN:
		return "TRUST_E_SUBJECT_FORM_UNKNOWN"
	default:
		return fmt.Sprintf("TRUST_E_ACTION_UNKNOWN %#x", ret)
	}

}
